# PostgreSQL (v2 storage)

This fork supports stable PostgreSQL 17.x and 18.x with UTF8 database and client encodings. SQLite remains the default.

## Configure a database

Create a dedicated database owned by the BirdNET-Go login role. The role must be able to create tables, indexes and constraints in the active schema. On a fresh database BirdNET-Go creates and seeds the v2 schema in one transaction. On restart it checks the v2 completion marker before opening the store. The connection pool allows up to 25 open connections and retains up to 10 idle connections.

Select the backend in `config.yaml`, set `BIRDNET_POSTGRESQL_PASSWORD` in the service environment if the login requires a password, then restart BirdNET-Go. Replace the example hostname, database, role and certificate path with your own.

```yaml
output:
  sqlite:
    enabled: false
  mysql:
    enabled: false
  postgresql:
    enabled: true
    host: postgres.example.net
    port: "5432"
    username: birdnet
    database: birdnet
    sslmode: verify-full
    sslrootcert: /etc/birdnet-go/certs/postgres-ca.crt
```

The default TLS mode, `verify-full`, checks the server's certificate chain and hostname. For client-certificate authentication, set both `sslcert` and `sslkey` to files readable by the application. The settings page shows the configured engine. Change PostgreSQL connection settings in the configuration file or service environment.

## Compatibility and diagnostics

At startup BirdNET-Go reads `server_version`, `server_version_num`, `server_encoding` and `client_encoding` on the same PostgreSQL connection it will use to initialize the schema. It checks the reported release against the tested major versions and requires UTF8 for both encodings. An unsupported major, a prerelease, inconsistent version information or an incompatible encoding stops startup before schema changes or seed writes, including when opening an existing v2 database.

If startup refuses the connection, check the reported version and both encodings in the service log. Select a supported stable major and configure the client connection for UTF8. A database created with another encoding needs an explicit migration to a UTF8 database. BirdNET-Go preserves the source and does not change its encoding. To inspect the settings using the application login, run:

```sql
SELECT current_setting('server_version'), current_setting('server_version_num'),
       current_setting('server_encoding'), current_setting('client_encoding');
```

In **System → Health**, run diagnostics to see **Database Server Version** and **Database Character Set**. The startup log and the notification bell also show the server version and both encodings. The running service refreshes the bell status after a server change. If startup is refused, the service log remains available even though the web interface cannot start.

The **Database Size** diagnostic and the database dashboard report the occupied bytes of the connected PostgreSQL database, including its indexes. The dashboard groups detections by UTC hour and shows each hour in the browser’s time zone. Hover over a bar to see its date, local time and UTC offset, including during a clock change.

If the database cannot open at startup, the error identifies the relevant `output.postgresql` setting. For authentication failures, check `username`, `password` and the server’s authentication rules. For a missing database, check `database`. For a permissions error, check the configured role’s grants on the database, schema and existing tables.

## History and backups

Selecting an empty PostgreSQL database starts a new history. This fork has no SQLite-to-PostgreSQL transfer tool. The existing legacy-to-v2 migration operates within SQLite or MySQL/MariaDB, and `tools/dbexport` targets MySQL from legacy SQLite. Keep the original database and its backups when evaluating the new backend. Use PostgreSQL tools such as `pg_dump` and `pg_restore` for PostgreSQL backups. The in-app database download and Database Doctor operate on SQLite files.

## Reproduce the tests

Follow the repository's Go build prerequisites. Start an isolated development PostgreSQL server and give the **test role** the `CREATEDB` privilege. The test helper connects to its `postgres` maintenance database, creates a unique disposable database per test, and drops it during cleanup. Test connections use loopback and unencrypted transport by default. The application role only needs rights on its own database and schema.

```sh
export BIRDNET_PG_TEST_PORT=55432
# Defaults: BIRDNET_PG_TEST_HOST=127.0.0.1 and BIRDNET_PG_TEST_USER=$USER.
# Set BIRDNET_PG_TEST_PASSWORD in the environment if your test role needs it.
go test -tags integration,noembed,skipfrontend -race -count=1 \
  -run 'Postgres|PostgreSQL' ./internal/analysis ./internal/api/v2 ./internal/datastore/v2 ./internal/datastore/v2only
```

The suite exercises bootstrap, restart, connection loss, related detection data, upserts, pruning, alerts, time-zone analytics and database inspection. The local TLS check also confirmed an encrypted `verify-full` session and rejection of a mismatched hostname.

### Fork validation on 7 November 2026

| Engine                           | Development checks                                                                                                                                                                                                             |
| -------------------------------- | ------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------------ |
| PostgreSQL 17.11                 | 16 top-level Go tests across the manager, datastore, service and HTTP packages passed with the race detector. The HTTP tests used a disposable database and in-process routes for recent detections and the database overview. |
| SQLite 3.53.4                    | Existing Go datastore and v2-only tests, including time-zone analytics.                                                                                                                                                        |
| MySQL 8.4.13 and MariaDB 11.4.13 | The existing range analytics and threshold retry integration tests passed against both server images.                                                                                                                          |

To repeat the two server regression tests, run the following command with a local Docker test environment and a supported image. Repeat with `MYSQL_TEST_IMAGE=mariadb:11.4`.

```sh
MYSQL_TEST_IMAGE=mysql:8.4 go test -tags integration,noembed,skipfrontend -race -count=1 \
  -run '^Test(V2OnlyDatastore_RangeAnalyticsWallClock|BatchSaveDynamicThresholds_RepeatAfterLostCommit)_MySQL$' ./internal/datastore/v2only
```
