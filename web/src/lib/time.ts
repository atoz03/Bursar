import { authState } from "./authStore";

function effectiveServerOffsetMinutes(): number {
  // 产品约定：前端所有时间统一按北京时间（UTC+8）展示。
  void authState.serverTzOffsetMinutes;
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

  // RFC3339 / ISO with timezone information
  if (/z$/i.test(text) || /[+-]\d{2}:\d{2}$/.test(text) || /[+-]\d{4}$/.test(text)) {
    return new Date(text);
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
