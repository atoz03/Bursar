export const STRONG_PASSWORD_RULE_TEXT =
  "强密码规则：长度需 10-16 位，且至少包含 1 个大写字母、1 个小写字母、1 个数字、1 个特殊字符（如 !@#$%^&*_-+=），且不能包含空格。";

export function checkStrongPassword(password: string): string | null {
  const text = String(password || "");
  if (text.length < 10 || text.length > 16) {
    return STRONG_PASSWORD_RULE_TEXT;
  }
  if (/\s/.test(text)) {
    return "密码不能包含空格";
  }
  if (!/[A-Z]/.test(text)) {
    return STRONG_PASSWORD_RULE_TEXT;
  }
  if (!/[a-z]/.test(text)) {
    return STRONG_PASSWORD_RULE_TEXT;
  }
  if (!/[0-9]/.test(text)) {
    return STRONG_PASSWORD_RULE_TEXT;
  }
  if (!/[^A-Za-z0-9]/.test(text)) {
    return STRONG_PASSWORD_RULE_TEXT;
  }
  return null;
}
