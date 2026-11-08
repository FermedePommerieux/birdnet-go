package analysis

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"sync"
	"time"

	"gorm.io/gorm"

	"github.com/tphakala/birdnet-go/internal/datastore/dbserver"
	"github.com/tphakala/birdnet-go/internal/logger"
	"github.com/tphakala/birdnet-go/internal/notification"
	"github.com/tphakala/birdnet-go/internal/telemetry"
)

const (
	// databaseServerProbeTimeout bounds the startup probe of the database server.
	databaseServerProbeTimeout = 5 * time.Second
	// databaseServerNoticeRetryDelay is the first wait before looking for the
	// notification service again; each retry doubles it.
	databaseServerNoticeRetryDelay = 30 * time.Second
	// databaseServerNoticeMaxRetries bounds the waits for the notification service.
	databaseServerNoticeMaxRetries = 4
	// databaseServerCheckOperation is the operation field of the startup log lines.
	databaseServerCheckOperation  = "database_server_check"
	postgresNoticeRefreshInterval = 5 * time.Minute
	postgresNoticeSignaturePrefix = "postgres|"
	postgresHealthyTitleKey       = "notifications.content.database.postgresHealthyTitle"
	postgresHealthyMessageKey     = "notifications.content.database.postgresHealthyMessage"
	postgresAttentionMessageKey   = "notifications.content.database.postgresAttentionMessage"
	postgresNoticeMetadataKey     = "database_server_postgresql"

	// Reasons that make up the signature of the bell notice.
	databaseServerReasonCharset     = "charset"
	databaseServerReasonUnsupported = "unsupported"
	databaseServerReasonSeparator   = "|"

	// databaseServerNoticeTitle and databaseServerNoticeMessage are the English
	// fallback text of the bell notice; the UI translates it by key.
	databaseServerNoticeTitle   = "Database server needs attention"
	databaseServerNoticeMessage = "The MySQL or MariaDB server is not a supported release, or the database does not use the utf8mb4 character set. Open System > Health for details."
)

// databaseServerReport is what one probe of the database server found.
type databaseServerReport struct {
	assessment dbserver.Assessment
	versionErr error
	charset    dbserver.CharsetReport
	charsetErr error
}

// databaseServerProbe reads the server version and character sets through db.
type databaseServerProbe func(ctx context.Context, db *gorm.DB) databaseServerReport

// probeDatabaseServer is the real databaseServerProbe. It only reads.
func probeDatabaseServer(ctx context.Context, db *gorm.DB) databaseServerReport {
	var r databaseServerReport
	raw, err := dbserver.QueryVersion(ctx, db)
	if err != nil {
		r.versionErr = err
	} else {
		r.assessment = dbserver.AssessRaw(raw)
	}
	r.charset, r.charsetErr = dbserver.QueryCharset(ctx, db)
	return r
}

// reasons returns the sorted reasons the bell notice is due, empty when none.
// An untracked or unparseable version is a note, not a problem, and a failed
// probe is not evidence of a problem. A schema default other than utf8mb4 is
// no reason while every table is utf8mb4.
func (r *databaseServerReport) reasons() []string {
	var reasons []string
	if r.versionErr == nil {
		switch r.assessment.Level {
		case dbserver.SupportUnsupported:
			reasons = append(reasons, databaseServerReasonUnsupported)
		case dbserver.SupportSupported, dbserver.SupportUntracked, dbserver.SupportUnknown:
		}
	}
	if r.charsetErr == nil && r.charset.SchemaCharset != "" && !r.charset.AllUTF8MB4() && !r.charset.EveryTableUTF8MB4() {
		reasons = append(reasons, databaseServerReasonCharset)
	}
	slices.Sort(reasons)
	return reasons
}

// tagValues returns the bounded telemetry values: server series, support level
// and character set class. A failed probe yields "unknown" for its values.
func (r *databaseServerReport) tagValues() (server, support, charset string) {
	server, support, charset = dbserver.TagUnknown, dbserver.TagUnknown, dbserver.TagUnknown
	if r.versionErr == nil {
		server = dbserver.ServerTag(&r.assessment)
		support = string(r.assessment.Level)
	}
	if r.charsetErr == nil {
		charset = dbserver.CharsetTag(r.charset)
	}
	return server, support, charset
}

// databaseServerNotice runs once per boot: it logs what the probe found, sets
// the telemetry tags and raises one bell notice when the server needs
// attention. It does not poll; the server release does not change under a
// running process often enough to justify it, and the health checks read live.
type databaseServerNotice struct {
	log logger.Logger

	// service returns the notification service, or nil while it is not up.
	// Tests replace it.
	service func() notification.NoticeService
	// setTags publishes the telemetry tags; tests replace it.
	setTags func(server, support, charset string) bool
	// retryDelay is the first wait for the notification service; zero means
	// databaseServerNoticeRetryDelay.
	retryDelay time.Duration

	// mu guards the fields below and serializes reconcile. Lock order: mu, then
	// the latch's own lock.
	mu           sync.Mutex
	latch        notification.PersistentNotice
	sig          string
	build        func() *notification.Notification
	timer        *time.Timer
	refreshTimer *time.Timer
	retries      int
	stopped      bool
	pruned       bool
}

// newDatabaseServerNotice creates the notice runner.
func newDatabaseServerNotice(log logger.Logger) *databaseServerNotice {
	n := &databaseServerNotice{
		log:     log,
		service: notification.DefaultNoticeService,
		setTags: telemetry.SetDatabaseServerTags,
	}
	// A failed create is retried by the latch on its own timer.
	n.latch.SetRetry(n.reconcile)
	return n
}

// report logs the probe result, publishes the tags and raises the bell notice
// when due. It never fails the caller.
func (n *databaseServerNotice) report(r *databaseServerReport) {
	n.logReport(r)
	server, support, charset := r.tagValues()
	_ = n.setTags(server, support, charset)

	reasons := r.reasons()
	n.mu.Lock()
	n.sig = strings.Join(reasons, databaseServerReasonSeparator)
	n.build = newDatabaseServerNotification
	n.mu.Unlock()
	if len(reasons) > 0 {
		n.reconcile()
	}
}

// reportPostgreSQL records the actual settings and keeps one bell entry for the
// current server state. An incompatible server replaces the informational entry.
func (n *databaseServerNotice) reportPostgreSQL(r dbserver.PostgreSQLReport) {
	sig := fmt.Sprintf("%s%s|%d|%s|%s|%t", postgresNoticeSignaturePrefix,
		r.Version, r.VersionNum, r.ServerEncoding, r.ClientEncoding,
		!r.VersionSupported() || !r.EncodingSupported())
	n.mu.Lock()
	unchanged := n.sig == sig
	n.mu.Unlock()
	if unchanged {
		n.reconcile()
		return
	}
	n.log.Info("PostgreSQL database server",
		logger.String("version", r.Version),
		logger.Int("version_num", r.VersionNum),
		logger.String("server_encoding", r.ServerEncoding),
		logger.String("client_encoding", r.ClientEncoding),
		logger.String("operation", databaseServerCheckOperation))
	var problems []string
	if !r.VersionSupported() {
		problems = append(problems, fmt.Sprintf("version %q is unsupported; use PostgreSQL %s", r.Version, dbserver.PostgreSQLSupportedVersions))
	}
	if !r.EncodingSupported() {
		problems = append(problems, fmt.Sprintf("server encoding %q and client encoding %q require UTF8; configure both as UTF8", r.ServerEncoding, r.ClientEncoding))
	}
	for _, problem := range problems {
		n.log.Warn("PostgreSQL server needs attention", logger.String("reason", problem), logger.String("operation", databaseServerCheckOperation))
	}
	message := fmt.Sprintf("PostgreSQL %s (server version number %d), server encoding %s, client encoding %s.",
		r.Version, r.VersionNum, r.ServerEncoding, r.ClientEncoding)
	params := map[string]any{
		"version": r.Version, "versionNum": r.VersionNum,
		"serverEncoding": r.ServerEncoding, "clientEncoding": r.ClientEncoding,
		"supportedVersions": dbserver.PostgreSQLSupportedVersions,
	}
	if len(problems) > 0 {
		message += " " + strings.Join(problems, "; ") + ". Open System > Health for details."
	}
	n.mu.Lock()
	n.sig = sig
	n.build = func() *notification.Notification {
		if len(problems) > 0 {
			return notification.NewNotification(notification.TypeWarning, notification.PriorityMedium,
				"PostgreSQL server needs attention", message).
				WithComponent(notification.ComponentDatabase).
				WithMetadata(postgresNoticeMetadataKey, true).
				WithTitleKey(notification.MsgDatabaseServerAttentionTitle, nil).
				WithMessageKey(postgresAttentionMessageKey, params).
				WithDeliveryTarget(notification.DeliveryTargetBell)
		}
		return notification.NewNotification(notification.TypeInfo, notification.PriorityLow,
			"PostgreSQL server", message).
			WithComponent(notification.ComponentDatabase).
			WithMetadata(postgresNoticeMetadataKey, true).
			WithTitleKey(postgresHealthyTitleKey, nil).
			WithMessageKey(postgresHealthyMessageKey, params).
			WithDeliveryTarget(notification.DeliveryTargetBell)
	}
	n.mu.Unlock()
	n.reconcile()
}

// pollPostgreSQL refreshes the one-shot state without writing to the database.
// A failed read logs the cause and leaves the last known notice in place.
func (n *databaseServerNotice) pollPostgreSQL(db *gorm.DB) {
	ctx, cancel := context.WithTimeout(context.Background(), databaseServerProbeTimeout)
	report, err := dbserver.ReadPostgreSQL(ctx, db)
	cancel()
	if err != nil {
		n.log.Warn("could not refresh PostgreSQL server settings", logger.Error(err),
			logger.String("operation", databaseServerCheckOperation))
	} else {
		n.reportPostgreSQL(report)
	}
	n.mu.Lock()
	if !n.stopped {
		n.refreshTimer = time.AfterFunc(postgresNoticeRefreshInterval, func() { n.pollPostgreSQL(db) })
	}
	n.mu.Unlock()
}

// logReport writes the INFO line and a WARN line for each reason.
func (n *databaseServerNotice) logReport(r *databaseServerReport) {
	if r.versionErr != nil {
		n.log.Warn("could not read the database server version",
			logger.Error(r.versionErr),
			logger.String("operation", databaseServerCheckOperation))
	}
	if r.charsetErr != nil {
		n.log.Warn("could not read the database character set",
			logger.Error(r.charsetErr),
			logger.String("operation", databaseServerCheckOperation))
	}
	if r.versionErr == nil {
		v := r.assessment.Version
		n.log.Info("database server",
			logger.String("vendor", string(v.Vendor)),
			logger.String("version", v.Raw),
			logger.String("series", v.Series()),
			logger.String("support", string(r.assessment.Level)),
			logger.String("schema_charset", r.charset.SchemaCharset),
			logger.String("operation", databaseServerCheckOperation))
	}
	for _, reason := range r.reasons() {
		n.log.Warn(attentionText(r, reason), logger.String("operation", databaseServerCheckOperation))
	}
}

// attentionText describes one reason for the startup WARN.
func attentionText(r *databaseServerReport, reason string) string {
	name := r.assessment.Version.Name()
	switch reason {
	case databaseServerReasonUnsupported:
		return fmt.Sprintf("%s is not a supported release; upgrade to a supported release (%s)",
			name, dbserver.SupportedReleases())
	default:
		return fmt.Sprintf("the database default character set is %s and %d table(s) are not utf8mb4; text outside that character set is rejected or stored incorrectly",
			r.charset.SchemaCharset, len(r.charset.NonUTF8MB4Tables()))
	}
}

// reconcile raises the bell notice for the current signature. Without a
// notification service it waits and tries again, a bounded number of times.
func (n *databaseServerNotice) reconcile() {
	n.mu.Lock()
	defer n.mu.Unlock()
	if n.stopped || n.sig == "" {
		return
	}
	svc := n.service()
	if svc == nil {
		n.armServiceRetryLocked()
		return
	}
	n.retries = 0
	if err := n.latch.Reconcile(svc, func() (string, func() *notification.Notification) {
		return n.sig, n.build
	}); err != nil {
		n.log.Warn("failed to raise the database server notification",
			logger.Error(err),
			logger.String("operation", databaseServerCheckOperation))
	} else if strings.HasPrefix(n.sig, postgresNoticeSignaturePrefix) && !n.pruned {
		n.pruned = n.prunePreviousPostgreSQLNotices(svc)
	}
}

// prunePreviousPostgreSQLNotices clears a PostgreSQL notice left by an earlier
// DatabaseService in this process. The notification store is a singleton and
// can outlive service restarts; its metadata identifies only our own entries.
func (n *databaseServerNotice) prunePreviousPostgreSQLNotices(svc notification.NoticeService) bool {
	lister, ok := svc.(interface {
		List(*notification.FilterOptions) ([]*notification.Notification, error)
	})
	if !ok {
		return true
	}
	items, err := lister.List(&notification.FilterOptions{Component: notification.ComponentDatabase})
	if err != nil {
		n.log.Warn("could not list prior PostgreSQL notices", logger.Error(err))
		return false
	}
	current := n.latch.ID()
	for _, item := range items {
		if item.ID == current || item.Metadata[postgresNoticeMetadataKey] != true {
			continue
		}
		if err := svc.Delete(item.ID); err != nil {
			n.log.Warn("could not remove prior PostgreSQL notice", logger.Error(err))
			return false
		}
	}
	return true
}

// armServiceRetryLocked schedules another reconcile while the notification
// service is missing, with a doubling delay, and gives up after
// databaseServerNoticeMaxRetries. The caller holds n.mu.
func (n *databaseServerNotice) armServiceRetryLocked() {
	if n.retries >= databaseServerNoticeMaxRetries {
		n.log.Debug("notification service unavailable, database server notice not raised",
			logger.String("operation", databaseServerCheckOperation))
		return
	}
	base := n.retryDelay
	if base <= 0 {
		base = databaseServerNoticeRetryDelay
	}
	delay := base << n.retries
	n.retries++
	n.timer = time.AfterFunc(delay, n.reconcile)
}

// newDatabaseServerNotification builds the bell notice.
func newDatabaseServerNotification() *notification.Notification {
	return notification.NewNotification(notification.TypeWarning, notification.PriorityMedium,
		databaseServerNoticeTitle, databaseServerNoticeMessage).
		WithComponent(notification.ComponentDatabase).
		WithTitleKey(notification.MsgDatabaseServerAttentionTitle, nil).
		WithMessageKey(notification.MsgDatabaseServerAttentionMessage, nil).
		WithDeliveryTarget(notification.DeliveryTargetBell)
}

// stop cancels pending retries and stops the latch so nothing more is raised.
// A notice already raised stays. It is idempotent.
func (n *databaseServerNotice) stop() {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.stopped = true
	if n.timer != nil {
		n.timer.Stop()
		n.timer = nil
	}
	if n.refreshTimer != nil {
		n.refreshTimer.Stop()
		n.refreshTimer = nil
	}
	n.latch.Stop()
}
