const BEIJING_OFFSET_MINUTES = 8 * 60;
const ZONED_DATE_TIME_RE =
  /^(\d{4})-(\d{2})-(\d{2})[ t](\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,3}))?(?:z|[+-]\d{2}:?\d{2})$/i;
const NAIVE_DATE_TIME_RE =
  /^(\d{4})-(\d{2})-(\d{2})[ t](\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,3}))?$/i;
const NAIVE_DATE_RE = /^(\d{4})-(\d{2})-(\d{2})$/;

type DateParts = {
  year: number;
  month: number;
  day: number;
};

type DateTimeParts = DateParts & {
  hour: number;
  minute: number;
  second: number;
  millis: number;
};

function pad2(v: number): string {
  return String(v).padStart(2, "0");
}

function pad4(v: number): string {
  return String(v).padStart(4, "0");
}

function parseNaiveDateTimeParts(text: string): DateTimeParts | null {
  const m = text.match(NAIVE_DATE_TIME_RE);
  if (!m) return null;
  return {
    year: Number(m[1]),
    month: Number(m[2]),
    day: Number(m[3]),
    hour: Number(m[4]),
    minute: Number(m[5]),
    second: Number(m[6]),
    millis: Number(String(m[7] || "").padEnd(3, "0") || 0),
  };
}

function parseNaiveDateParts(text: string): DateParts | null {
  const m = text.match(NAIVE_DATE_RE);
  if (!m) return null;
  return {
    year: Number(m[1]),
    month: Number(m[2]),
    day: Number(m[3]),
  };
}

function buildBeijingEpochMs(parts: DateTimeParts): number {
  return (
    Date.UTC(parts.year, parts.month - 1, parts.day, parts.hour, parts.minute, parts.second, parts.millis) -
    BEIJING_OFFSET_MINUTES * 60_000
  );
}

function formatBeijingDateTimeParts(parts: DateTimeParts): string {
  return `${pad4(parts.year)}-${pad2(parts.month)}-${pad2(parts.day)} ${pad2(parts.hour)}:${pad2(parts.minute)}:${pad2(parts.second)}`;
}

function formatBeijingDateParts(parts: DateParts): string {
  return `${pad4(parts.year)}-${pad2(parts.month)}-${pad2(parts.day)}`;
}

function formatBeijingEpochMs(ms: number): DateTimeParts {
  const shifted = new Date(ms + BEIJING_OFFSET_MINUTES * 60_000);
  return {
    year: shifted.getUTCFullYear(),
    month: shifted.getUTCMonth() + 1,
    day: shifted.getUTCDate(),
    hour: shifted.getUTCHours(),
    minute: shifted.getUTCMinutes(),
    second: shifted.getUTCSeconds(),
    millis: shifted.getUTCMilliseconds(),
  };
}

function parseInputEpochMs(input: string | number | Date): number {
  if (input instanceof Date) return input.getTime();
  if (typeof input === "number") return input;
  const text = String(input || "").trim();
  if (!text) return Number.NaN;

  if (ZONED_DATE_TIME_RE.test(text)) {
    return new Date(text).getTime();
  }

  const naiveDateTime = parseNaiveDateTimeParts(text);
  if (naiveDateTime) {
    return buildBeijingEpochMs(naiveDateTime);
  }

  const naiveDate = parseNaiveDateParts(text);
  if (naiveDate) {
    return buildBeijingEpochMs({
      ...naiveDate,
      hour: 0,
      minute: 0,
      second: 0,
      millis: 0,
    });
  }

  return new Date(text).getTime();
}

export function toServerEpochMs(input: string | number | Date | null | undefined): number {
  if (input === null || input === undefined || input === "") return NaN;
  return parseInputEpochMs(input);
}

export function formatServerDateTime(input: string | number | Date | null | undefined): string {
  if (input === null || input === undefined || input === "") return "-";
  if (typeof input === "string") {
    const text = String(input || "").trim();
    const naiveDateTime = parseNaiveDateTimeParts(text);
    if (naiveDateTime) {
      return formatBeijingDateTimeParts(naiveDateTime);
    }
  }
  const ms = parseInputEpochMs(input);
  if (!Number.isFinite(ms)) return String(input);
  return formatBeijingDateTimeParts(formatBeijingEpochMs(ms));
}

export function formatServerDate(input: string | number | Date | null | undefined): string {
  if (input === null || input === undefined || input === "") return "-";
  if (typeof input === "string") {
    const text = String(input || "").trim();
    const naiveDate = parseNaiveDateParts(text);
    if (naiveDate) {
      return formatBeijingDateParts(naiveDate);
    }
    const naiveDateTime = parseNaiveDateTimeParts(text);
    if (naiveDateTime) {
      return formatBeijingDateParts(naiveDateTime);
    }
  }
  const ms = parseInputEpochMs(input);
  if (!Number.isFinite(ms)) return String(input);
  return formatBeijingDateParts(formatBeijingEpochMs(ms));
}

export function formatServerHMS(input: string | number | Date | null | undefined): string {
  if (input === null || input === undefined || input === "") return "-";
  if (typeof input === "string") {
    const text = String(input || "").trim();
    const naiveDateTime = parseNaiveDateTimeParts(text);
    if (naiveDateTime) {
      return `${pad2(naiveDateTime.hour)}:${pad2(naiveDateTime.minute)}:${pad2(naiveDateTime.second)}`;
    }
  }
  const ms = parseInputEpochMs(input);
  if (!Number.isFinite(ms)) return String(input);
  const parts = formatBeijingEpochMs(ms);
  return `${pad2(parts.hour)}:${pad2(parts.minute)}:${pad2(parts.second)}`;
}

export function getServerTodayDateText(): string {
  return formatServerDate(Date.now());
}

export function getServerCurrentYear(): number {
  return formatBeijingEpochMs(Date.now()).year;
}

export function shiftServerDateText(base: string, days: number): string {
  const startMs = toServerDateStartEpochMs(base);
  if (!Number.isFinite(startMs)) return base;
  return formatServerDate(startMs + days * 24 * 60 * 60 * 1000);
}

export function normalizeServerDateInput(v: unknown, fallback: string): string {
  if (typeof v === "string") {
    const s = String(v || "").trim();
    if (/^\d{4}-\d{2}-\d{2}$/.test(s)) return s;
    if (!s) return fallback;
    const out = formatServerDate(s);
    return out === "-" ? fallback : out;
  }
  if (v instanceof Date || typeof v === "number") {
    const out = formatServerDate(v);
    return out === "-" ? fallback : out;
  }
  return fallback;
}

export function toServerDateStartEpochMs(v: string): number {
  const s = String(v || "").trim();
  const date = parseNaiveDateParts(s);
  if (!date) return Number.NaN;
  return buildBeijingEpochMs({ ...date, hour: 0, minute: 0, second: 0, millis: 0 });
}

export function toServerDateEndEpochMs(v: string): number {
  const startMs = toServerDateStartEpochMs(v);
  if (!Number.isFinite(startMs)) return Number.NaN;
  return startMs + 24 * 60 * 60 * 1000 - 1;
}
