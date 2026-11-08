// Format the browser-local time with its UTC offset. The offset distinguishes
// the two occurrences of an hour during an autumn daylight-saving transition.
export function createDetectionRateTimeFormatter(timeZone?: string, locale?: string) {
  const formatter = new Intl.DateTimeFormat(locale, {
    year: 'numeric',
    month: '2-digit',
    day: '2-digit',
    hour: '2-digit',
    minute: '2-digit',
    hourCycle: 'h23',
    timeZone,
    timeZoneName: 'shortOffset',
  });
  return (isoHour: string): string => formatter.format(new Date(isoHour));
}
