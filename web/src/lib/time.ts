function effectiveServerOffsetMinutes(): number {
  return 8 * 60;
}

function pad2(v: number): string {
  return String(v).padStart(2, "0");
}

function pad4(v: number): string {
  return String(v).padStart(4, "0");
}

function parseServerTimeInput(input: string | number | Date): Date {
  if (input instanceof Date) return input;
  if (typeof input === "number") return new Date(input);
  const text = String(input || "").trim();
  if (!text) return new Date(NaN);

  // RFC3339 / ISO with timezone information:
  // 该项目后端历史上存在“带时区后缀但语义是服务器墙上时间”的数据，
  // 这里统一按服务器本地时间解释，避免再次发生 8 小时偏差。
  const zoned = text.match(
    /^(\d{4})-(\d{2})-(\d{2})[ t](\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,3}))?(?:z|[+-]\d{2}:?\d{2})$/i,
  );
  if (zoned) {
    const year = Number(zoned[1]);
    const month = Number(zoned[2]) - 1;
    const day = Number(zoned[3]);
    const hour = Number(zoned[4]);
    const minute = Number(zoned[5]);
    const second = Number(zoned[6]);
    const millis = Number(String(zoned[7] || "").padEnd(3, "0") || 0);
    const offsetMinutes = effectiveServerOffsetMinutes();
    const utcMs = Date.UTC(year, month, day, hour, minute, second, millis) - offsetMinutes * 60_000;
    return new Date(utcMs);
  }

  // "YYYY-MM-DD HH:mm:ss(.sss)" / "YYYY-MM-DDTHH:mm:ss(.sss)" without timezone:
  // treat as server-local time (not browser-local time), then convert to UTC instant.
  const dt = text.match(
    /^(\d{4})-(\d{2})-(\d{2})[ t](\d{2}):(\d{2}):(\d{2})(?:\.(\d{1,3}))?$/i,
  );
  if (dt) {
    const year = Number(dt[1]);
    const month = Number(dt[2]) - 1;
    const day = Number(dt[3]);
    const hour = Number(dt[4]);
    const minute = Number(dt[5]);
    const second = Number(dt[6]);
    const millis = Number(String(dt[7] || "").padEnd(3, "0") || 0);
    const offsetMinutes = effectiveServerOffsetMinutes();
    const utcMs = Date.UTC(year, month, day, hour, minute, second, millis) - offsetMinutes * 60_000;
    return new Date(utcMs);
  }

  // "YYYY-MM-DD" without timezone: treat as server-local midnight.
  const d = text.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (d) {
    const year = Number(d[1]);
    const month = Number(d[2]) - 1;
    const day = Number(d[3]);
    const offsetMinutes = effectiveServerOffsetMinutes();
    const utcMs = Date.UTC(year, month, day, 0, 0, 0, 0) - offsetMinutes * 60_000;
    return new Date(utcMs);
  }

  return new Date(text);
}

function shiftToServerClock(instant: Date): Date {
  const offsetMinutes = effectiveServerOffsetMinutes();
  return new Date(instant.getTime() + offsetMinutes * 60_000);
}

export function toServerEpochMs(input: string | number | Date | null | undefined): number {
  if (input === null || input === undefined || input === "") return NaN;
  const d = parseServerTimeInput(input);
  return d.getTime();
}

export function formatServerDateTime(input: string | number | Date | null | undefined): string {
  if (input === null || input === undefined || input === "") return "-";
  const d = parseServerTimeInput(input);
  if (Number.isNaN(d.getTime())) return String(input);
  const shifted = shiftToServerClock(d);
  const yyyy = shifted.getUTCFullYear();
  const mon = shifted.getUTCMonth() + 1;
  const dd = shifted.getUTCDate();
  const hh = shifted.getUTCHours();
  const min = shifted.getUTCMinutes();
  const ss = shifted.getUTCSeconds();
  return `${pad4(yyyy)}-${pad2(mon)}-${pad2(dd)} ${pad2(hh)}:${pad2(min)}:${pad2(ss)}`;
}

export function formatServerDate(input: string | number | Date | null | undefined): string {
  if (input === null || input === undefined || input === "") return "-";
  const d = parseServerTimeInput(input);
  if (Number.isNaN(d.getTime())) return String(input);
  const shifted = shiftToServerClock(d);
  const yyyy = shifted.getUTCFullYear();
  const mon = shifted.getUTCMonth() + 1;
  const dd = shifted.getUTCDate();
  return `${pad4(yyyy)}-${pad2(mon)}-${pad2(dd)}`;
}

export function formatServerHMS(input: string | number | Date | null | undefined): string {
  if (input === null || input === undefined || input === "") return "-";
  const d = parseServerTimeInput(input);
  if (Number.isNaN(d.getTime())) return String(input);
  const shifted = shiftToServerClock(d);
  const hh = shifted.getUTCHours();
  const mm = shifted.getUTCMinutes();
  const ss = shifted.getUTCSeconds();
  return `${pad2(hh)}:${pad2(mm)}:${pad2(ss)}`;
}

export function getServerTodayDateText(): string {
  return formatServerDate(new Date());
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
  const m = s.match(/^(\d{4})-(\d{2})-(\d{2})$/);
  if (!m) return Number.NaN;
  const year = Number(m[1]);
  const month = Number(m[2]) - 1;
  const day = Number(m[3]);
  return Date.UTC(year, month, day, 0, 0, 0, 0) - effectiveServerOffsetMinutes() * 60_000;
}

export function toServerDateEndEpochMs(v: string): number {
  const startMs = toServerDateStartEpochMs(v);
  if (!Number.isFinite(startMs)) return Number.NaN;
  return startMs + 24 * 60 * 60 * 1000 - 1;
}
