<template>
  <div class="register-page">
    <div class="bg-layer">
      <div class="flow flow-a" />
      <div class="flow flow-b" />
      <div class="blob blob-a" />
      <div class="blob blob-b" />
      <div class="blob blob-c" />
      <div class="spark spark-a" />
      <div class="spark spark-b" />
      <div class="sticker sticker-a">审核制</div>
      <div class="sticker sticker-b">真实信息优先</div>
    </div>

    <el-card class="form-card">
      <div v-if="registerLocked" class="lock-overlay">
        <div class="lock-panel">
          <div class="lock-title">{{ t("注册申请已临时锁定", "Registration Temporarily Locked") }}</div>
          <div class="lock-count">{{ cooldownRemainingSeconds }}</div>
          <div class="lock-text">{{ cooldownMessage }}</div>
          <div class="lock-sub">{{ t("倒计时结束后将自动恢复，你无需刷新页面。", "The page will unlock automatically when the countdown ends.") }}</div>
        </div>
      </div>
      <template #header>
        <div class="head">
          <div class="head-top">
            <h2>{{ t("平台账号注册申请", "Platform Registration") }}</h2>
            <el-button text @click="toggleUiLanguage">{{ uiLocaleState.language === "en" ? "中" : "EN" }}</el-button>
          </div>
          <p>{{ t("先完成邮箱验证，再进入管理员审核，通过后会进行邮箱通知(注意查询邮箱垃圾箱)。", "Complete email verification first, then wait for admin review. Approval notices will be sent by email.") }}</p>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
      <el-alert
        v-if="verifiedSubmitted"
        :title="t('请在审核通过后第一时间填写你已有的计算节点账号（节点账号页面），否则系统可能无法识别你的使用记录。', 'After approval, add your existing node account mappings as soon as possible, otherwise your usage may not be recognized correctly.')"
        type="error"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-alert
        :title="t('所有字段都必填。用户名必须按“姓名缩写+学号”填写；用户名、学号、邮箱都不能和已注册账号、待审核申请或待验证申请重复。', 'All fields are required. Username must be initials + student ID, and username, student ID, and email must be unique across registered, pending-review, and pending-verification accounts.')"
        type="warning"
        :closable="false"
        show-icon
        class="mb"
      />
      <div class="rule-chips">
        <span class="chip chip-orange">{{ t("用户名：姓名缩写+学号", "Username: initials + student ID") }}</span>
        <span class="chip chip-cyan">{{ t("学号：全平台唯一", "Student ID: globally unique") }}</span>
        <span class="chip chip-blue">{{ t("邮箱：仅允许 @example.org / @students.example.org", "Email: only @example.org / @students.example.org") }}</span>
      </div>
      <div class="rule-note">{{ t("提交前会自动校验重复项；并强制校验“用户名=姓名缩写+学号”“邮箱前缀=学号（学号自动转大写）”。", "The form checks duplicates before submit, enforces username = initials + student ID, and email prefix = student ID (student ID is auto-uppercased).") }}</div>

      <el-form label-position="top" :disabled="submitted || registerLocked" class="register-form">
        <div class="section-grid">
          <section class="form-section section-account">
            <div class="section-head">
              <span class="section-dot dot-account" />
              <div>
                <h3>{{ t("账号信息", "Account Information") }}</h3>
                <p>{{ t("用于登录平台，注意唯一性。", "Used to sign in. Must be unique.") }}</p>
              </div>
            </div>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required :class="fieldClass('email')">
                  <template #label><span class="required">*</span> {{ t("邮箱", "Email") }}</template>
                  <el-input
                    v-model="form.email"
                    :placeholder="t('例如：26B123456@example.org', 'Example: 26B123456@example.org')"
                    @input="onEmailInput"
                    @blur="checkUnique('email')"
                  />
                  <div class="field-tip">{{ t("必须使用 `@example.org` 或 `@students.example.org`；邮箱前缀必须与学号一致。", "Use only @example.org or @students.example.org; the email prefix must match your student ID.") }}</div>
                  <div v-if="fieldErrors.email" class="field-error">{{ fieldErrors.email }}</div>
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required :class="fieldClass('username')">
                  <template #label><span class="required">*</span> {{ t("用户名", "Username") }}</template>
                  <el-input v-model="form.username" :placeholder="usernamePlaceholder" @input="onUsernameInput" @blur="checkUnique('username')" />
                  <div class="field-tip">{{ t("必须填写成“姓名拼音首字母缩写 + 学号”。例如：张三 -> zs；李小龙 -> lxl；最终用户名示例：", "Username must be initials + student ID. Example: Zhang San -> zs; Li Xiaolong -> lxl; final example: ") }}<code>{{ usernameExample }}</code></div>
                  <div class="field-tip">{{ t("前缀只能是 2-8 个小写字母，后缀必须与下方学号完全一致。", "The prefix must be 2-8 lowercase letters, and the suffix must exactly match the student ID below.") }}</div>
                  <div v-if="fieldErrors.username" class="field-error">{{ fieldErrors.username }}</div>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required :class="fieldClass('password')">
                  <template #label><span class="required">*</span> {{ t("密码", "Password") }}</template>
                  <el-input v-model="form.password" type="password" show-password :placeholder="t('请设置强密码', 'Choose a strong password')" @input="clearFieldError('password')" />
                  <div class="field-tip">{{ passwordRuleText }}</div>
                  <div v-if="fieldErrors.password" class="field-error">{{ fieldErrors.password }}</div>
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required :class="fieldClass('confirm_password')">
                  <template #label><span class="required">*</span> {{ t("确认密码", "Confirm Password") }}</template>
                  <el-input v-model="confirmPassword" type="password" show-password @input="clearFieldError('confirm_password')" />
                  <div v-if="fieldErrors.confirm_password" class="field-error">{{ fieldErrors.confirm_password }}</div>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item required :class="fieldClass('captcha')">
              <template #label><span class="required">*</span> {{ t("安全验证码（每次注册必做）", "Security Captcha (required for every registration)") }}</template>
              <div class="captcha-wrap">
                <div class="captcha-question">{{ captchaQuestionLabel }}</div>
                <el-radio-group v-model="captchaOption" class="captcha-options" @change="clearFieldError('captcha')">
                  <el-radio-button v-for="(op, idx) in captchaOptions" :key="`${captchaId}-${idx}-${op}`" :label="idx">{{ op }}</el-radio-button>
                </el-radio-group>
                <div class="captcha-actions">
                  <el-button text type="primary" :loading="captchaLoading" @click="loadCaptcha">{{ t("换一题", "Refresh") }}</el-button>
                </div>
              </div>
              <div v-if="fieldErrors.captcha" class="field-error">{{ fieldErrors.captcha }}</div>
            </el-form-item>
          </section>

          <section class="form-section section-profile">
            <div class="section-head">
              <span class="section-dot dot-profile" />
              <div>
                <h3>{{ t("身份信息", "Profile Information") }}</h3>
                <p>{{ t("请填写真实资料，便于管理员审核。", "Use real information to help admin review.") }}</p>
              </div>
            </div>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required :class="fieldClass('real_name')">
                  <template #label><span class="required">*</span> {{ t("真实姓名", "Real Name") }}</template>
                  <el-input v-model="form.real_name" :placeholder="t('请填写真实中文姓名，例如：张三', 'Enter your real name')" @input="clearFieldError('real_name')" />
                  <div class="field-tip">{{ t("请填写真实姓名；用户名里的“姓名缩写”请按这个姓名的拼音首字母自行填写。", "Enter your real name; the username initials should be based on this name.") }}</div>
                  <div v-if="fieldErrors.real_name" class="field-error">{{ fieldErrors.real_name }}</div>
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required :class="fieldClass('student_id')">
                  <template #label><span class="required">*</span> {{ t("学号", "Student ID") }}</template>
                  <el-input v-model="form.student_id" :placeholder="t('注意全大写，例如26B123456', 'Uppercase only, for example 26B123456')" @input="onStudentInput" @blur="checkUnique('student_id')" />
                  <div class="field-tip">{{ t("输入小写会自动转为大写；并将用于校验邮箱前缀。", "Lowercase input is auto-converted to uppercase and used to validate the email prefix.") }}</div>
                  <div v-if="fieldErrors.student_id" class="field-error">{{ fieldErrors.student_id }}</div>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required :class="fieldClass('advisor')">
                  <template #label><span class="required">*</span> {{ t("导师", "Advisor") }}</template>
                  <el-input v-model="form.advisor" @input="clearFieldError('advisor')" />
                  <div v-if="fieldErrors.advisor" class="field-error">{{ fieldErrors.advisor }}</div>
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required :class="fieldClass('expected_graduation')">
                  <template #label><span class="required">*</span> {{ t("预计毕业时间（年-月）", "Expected Graduation (YYYY-MM)") }}</template>
                  <el-date-picker
                    v-model="graduationYm"
                    type="month"
                    value-format="YYYY-MM"
                    format="YYYY-MM"
                    style="width: 100%"
                    :placeholder="t('请选择毕业年月', 'Select graduation month')"
                    @change="clearFieldError('expected_graduation')"
                  />
                  <div v-if="fieldErrors.expected_graduation" class="field-error">{{ fieldErrors.expected_graduation }}</div>
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item required :class="fieldClass('phone')">
              <template #label><span class="required">*</span> {{ t("电话", "Phone") }}</template>
              <el-input v-model="form.phone" @input="clearFieldError('phone')" />
              <div v-if="fieldErrors.phone" class="field-error">{{ fieldErrors.phone }}</div>
            </el-form-item>
          </section>
        </div>
      </el-form>

      <div :class="agreeLineClass()">
        <el-checkbox v-model="acceptGuideline" :disabled="submitted || registerLocked" @change="clearFieldError('accept_guideline')">
          {{ t("我已阅读并同意", "I have read and agree to the") }}
          <button type="button" class="guideline-link" @click.prevent="guidelineVisible = true">{{ t("《用户准则》", "User Guidelines") }}</button>
          {{ t("，自觉遵守平台规范，否则后果自负。", "and will comply with the platform rules.") }}
        </el-checkbox>
        <div v-if="fieldErrors.accept_guideline" class="field-error">{{ fieldErrors.accept_guideline }}</div>
      </div>

      <el-button
        :type="submitted ? 'info' : 'primary'"
        :loading="loading"
        :disabled="submitted || registerLocked"
        @click="submit"
        class="submit-btn"
      >
        {{ verifiedSubmitted ? t("已提交，等待审核", "Submitted, waiting for review") : submitted ? t("验证邮件已发送", "Verification email sent") : t("提交注册申请", "Submit Registration") }}
      </el-button>

      <div class="links">
        <router-link to="/login">{{ t("返回登录", "Back to Login") }}</router-link>
      </div>
    </el-card>

    <el-dialog v-model="guidelineVisible" :title="t('用户准则', 'User Guidelines')" width="760px">
      <div class="guideline-wrap">
        <div class="md-body" v-html="renderMarkdown(guidelineContent)" />
      </div>
      <template #footer>
        <el-button @click="guidelineVisible = false">{{ t("关闭", "Close") }}</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, nextTick, onBeforeUnmount, onMounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { ElMessageBox } from "element-plus";
import { renderMarkdown } from "../../lib/markdown";
import { STRONG_PASSWORD_RULE_TEXT, checkStrongPassword } from "../../lib/passwordPolicy";
import { getServerCurrentYear } from "../../lib/time";
import { pickText, toggleUiLanguage, uiLocaleState } from "../../lib/uiLocale";

type FieldKey =
  | "username"
  | "email"
  | "student_id"
  | "password"
  | "confirm_password"
  | "real_name"
  | "advisor"
  | "expected_graduation"
  | "phone"
  | "accept_guideline"
  | "captcha";
const allowedEmailDomains = ["example.org", "students.example.org"];
const MAX_FRONTEND_REGISTER_COOLDOWN_SECONDS = 1000;

function toYYYYMM(year: number, month: number): string {
  return `${year}-${String(month).padStart(2, "0")}`;
}

function parseYYYYMM(v: string): { year: number; month: number } | null {
  const m = (v || "").match(/^(\d{4})-(\d{2})$/);
  if (!m) return null;
  const year = Number(m[1]);
  const month = Number(m[2]);
  if (year < 2000 || year > 2200 || month < 1 || month > 12) return null;
  return { year, month };
}

const loading = ref(false);
const submitted = ref(false);
const verifiedSubmitted = ref(false);
const error = ref("");
const success = ref("");
const confirmPassword = ref("");
const cooldownRemainingSeconds = ref(0);
const cooldownMessage = ref("");
const defaultGraduationYear = getServerCurrentYear() + 3;
const graduationYm = ref(toYYYYMM(defaultGraduationYear, 6));
const guidelineVisible = ref(false);
const guidelineContent = ref("正在加载用户准则...");
const acceptGuideline = ref(false);
const passwordRuleText = STRONG_PASSWORD_RULE_TEXT;
const route = useRoute();
const router = useRouter();
const captchaLoading = ref(false);
const captchaId = ref("");
const captchaQuestion = ref("");
const captchaOptions = ref<number[]>([]);
const captchaOption = ref<number | null>(null);
let cooldownTimer: ReturnType<typeof setInterval> | null = null;
const captchaQuestionLabel = computed(() => captchaQuestion.value || t("验证码加载中...", "Loading captcha..."));
const registerLocked = computed(() => cooldownRemainingSeconds.value > 0);
const usernameExample = computed(() => {
  const student = normalizeStudentIDInput(form.student_id);
  return `zs${student || "26B123456"}`;
});
const usernamePlaceholder = computed(() =>
  t(`例如：${usernameExample.value}（姓名缩写+学号）`, `Example: ${usernameExample.value} (initials + student ID)`),
);
const fieldErrors = reactive<Record<FieldKey, string>>({
  username: "",
  email: "",
  student_id: "",
  password: "",
  confirm_password: "",
  real_name: "",
  advisor: "",
  expected_graduation: "",
  phone: "",
  accept_guideline: "",
  captcha: "",
});
const form = reactive({
  email: "",
  username: "",
  password: "",
  real_name: "",
  student_id: "",
  advisor: "",
  expected_graduation_year: defaultGraduationYear,
  expected_graduation_month: 6,
  phone: "",
});

function t(zh: string, en: string): string {
  return pickText(zh, en);
}

function normalizeStudentIDInput(v: string): string {
  return String(v || "").trim().toUpperCase();
}

function clearFieldError(field: FieldKey) {
  fieldErrors[field] = "";
}

function clearAllFieldErrors() {
  (Object.keys(fieldErrors) as FieldKey[]).forEach((k) => {
    fieldErrors[k] = "";
  });
}

function fieldClass(field: FieldKey): string {
  return fieldErrors[field] ? "field-item-invalid" : "";
}

function agreeLineClass(): string {
  return fieldErrors.accept_guideline ? "agree-line agree-line-invalid" : "agree-line";
}

function revealFirstInvalidField() {
  void nextTick(() => {
    const el = document.querySelector(".field-item-invalid, .agree-line-invalid") as HTMLElement | null;
    if (!el) return;
    el.scrollIntoView({ behavior: "smooth", block: "center" });
  });
}

function clampFrontendCooldownSeconds(seconds: number): number {
  return Math.max(1, Math.min(MAX_FRONTEND_REGISTER_COOLDOWN_SECONDS, Math.ceil(Number(seconds || 0))));
}

function normalizeRegisterEmailForStudent(emailRaw: string, studentRaw: string): { value: string; error: string | null } {
  const student = normalizeStudentIDInput(studentRaw);
  const email = String(emailRaw || "").trim();
  if (!email) return { value: "", error: "邮箱不能为空" };
  if (!student) return { value: email, error: "请先填写学号" };
  const m = email.match(/^([^@\s]+)@([^@\s]+)$/);
  if (!m) return { value: email, error: "邮箱格式不合法" };
  const local = String(m[1] || "").trim().toUpperCase();
  const domain = String(m[2] || "").trim().toLowerCase();
  if (!allowedEmailDomains.includes(domain)) {
    return { value: email, error: "注册邮箱后缀仅支持 @example.org 或 @students.example.org" };
  }
  if (local !== student) {
    return { value: `${student}@${domain}`, error: "邮箱前缀必须与学号一致（邮箱前缀=学号）" };
  }
  return { value: `${student}@${domain}`, error: null };
}

function validateUsernameRuleLocal(usernameRaw: string, studentRaw: string): string {
  const username = String(usernameRaw || "").trim();
  const student = normalizeStudentIDInput(studentRaw);
  if (!username) return "用户名不能为空";
  if (!student) return "请先填写学号，再按“姓名缩写+学号”填写用户名。";
  if (Array.from(username).length > 18) return "用户名最多 18 个字符，请缩短后再试。";
  if (!username.endsWith(student)) return `用户名必须以学号 ${student} 结尾，例如 ${usernameExample.value}。`;
  const prefix = username.slice(0, username.length - student.length);
  if (!/^[a-z]{2,8}$/.test(prefix)) {
    return `用户名必须写成“姓名缩写+学号”，前缀需为 2-8 个小写字母，例如 ${usernameExample.value}。`;
  }
  return "";
}

function onStudentInput() {
  form.student_id = normalizeStudentIDInput(form.student_id);
  clearFieldError("student_id");
  if (String(form.username || "").trim()) {
    fieldErrors.username = validateUsernameRuleLocal(form.username, form.student_id);
  }
  const emailRaw = String(form.email || "").trim();
  if (emailRaw) {
    const normalized = normalizeRegisterEmailForStudent(emailRaw, form.student_id);
    form.email = normalized.value;
    fieldErrors.email = normalized.error || "";
  }
}

function onEmailInput() {
  clearFieldError("email");
}

function onUsernameInput() {
  fieldErrors.username = validateUsernameRuleLocal(form.username, form.student_id);
}

function firstFieldErrorMessage(): string {
  for (const key of Object.keys(fieldErrors) as FieldKey[]) {
    const msg = String(fieldErrors[key] || "").trim();
    if (msg) return msg;
  }
  return "请根据表单提示修改后再提交。";
}

function normalizeFieldError(field: FieldKey, msg: string): string {
  const text = String(msg || "").trim();
  if (!text) return "";
  if (text === "用户名已存在") return "该用户名已被使用，请换一个用户名。";
  if (text === "邮箱已存在") return "该邮箱已被使用，请换一个邮箱。";
  if (text === "邮箱格式不合法") return "邮箱格式不正确，请检查后再填写。";
  if (text.includes("注册邮箱后缀仅支持")) return "邮箱后缀仅支持 @example.org 或 @students.example.org。";
  if (text.includes("邮箱前缀必须与学号一致")) return "邮箱前缀必须与学号一致（邮箱前缀=学号）。";
  if (text === "请先填写学号") return "请先填写学号。";
  if (text === "学号已存在") return "该学号已被使用，请确认后再填写。";
  if (text === "用户名已被待审核申请占用") return "该用户名已被他人提交注册申请，请更换。";
  if (text === "邮箱已被待审核申请占用") return "该邮箱已被他人提交注册申请，请更换。";
  if (text === "学号已被待审核申请占用") return "该学号已被他人提交注册申请，请更换。";
  if (text === "用户名已被待验证申请占用") return "该用户名已有待验证申请，请先完成邮箱验证或稍后重试。";
  if (text === "邮箱已被待验证申请占用") return "该邮箱已有待验证申请，请先完成邮箱验证或稍后重试。";
  if (text === "学号已被待验证申请占用") return "该学号已有待验证申请，请先完成邮箱验证或稍后重试。";
  if (text === "用户名不能为空") return "用户名不能为空。";
  if (text === "请先填写学号，再按“姓名缩写+学号”格式填写用户名") return "请先填写学号，再按“姓名缩写+学号”格式填写用户名。";
  if (text.startsWith("用户名必须以学号 ")) return `${text}。`;
  if (text.includes("用户名必须写成“姓名缩写+学号”")) return `${text}。`;
  if (field === "username" && text.includes("18")) return "用户名最多 18 个字符，请缩短后再试。";
  return text;
}

function normalizeRegisterError(msg: string): string {
  const text = String(msg || "").trim();
  if (!text) return "提交失败，请稍后重试。";
  if (text.startsWith("以下信息不可用：")) {
    const fieldsRaw = text.replace("以下信息不可用：", "").trim();
    const fields = fieldsRaw
      .split(/[、，,]/g)
      .map((x) => x.trim())
      .filter(Boolean)
      .join("、");
    return `提交失败：你填写的${fields || "信息"}已被占用，请修改后再提交。`;
  }
  if (text.startsWith("以下信息已存在账号：")) {
    const fieldsRaw = text.replace("以下信息已存在账号：", "").trim();
    const fields = fieldsRaw
      .split(/[、，,]/g)
      .map((x) => x.trim())
      .filter(Boolean)
      .join("、");
    return `提交失败：你填写的${fields || "信息"}已被占用，请修改后再提交。`;
  }
  if (text === "请完整填写注册信息") return "请把所有必填项填写完整后再提交。";
  if (text === "用户名不能为空") return "用户名不能为空。";
  if (text === "请先填写学号，再按“姓名缩写+学号”格式填写用户名") return "请先填写学号，再按“姓名缩写+学号”格式填写用户名。";
  if (text === "用户名不得超过 18 个字符") return "用户名最多 18 个字符，请缩短后再提交。";
  if (text.startsWith("用户名必须以学号 ")) return `${text}。`;
  if (text.includes("用户名必须写成“姓名缩写+学号”")) return `${text}。`;
  if (text.includes("强密码规则")) return STRONG_PASSWORD_RULE_TEXT;
  if (text === "密码不能包含空格") return "密码不能包含空格。";
  if (text === "邮箱格式不合法") return "邮箱格式不正确，请检查后再提交。";
  if (text.includes("注册邮箱后缀仅支持")) return "注册邮箱后缀仅支持 @example.org 或 @students.example.org。";
  if (text.includes("邮箱前缀必须与学号一致")) return "邮箱前缀必须与学号一致（邮箱前缀=学号）。";
  if (text === "请求过于频繁，请稍后再试") return "请求过于频繁，请稍后再试。";
  if (text === "该邮箱请求过于频繁，请稍后再试") return "该邮箱请求过于频繁，请稍后再试。";
  if (text === "验证码错误，请重试") return "验证码错误，请重新选择后再提交。";
  if (text === "验证码已过期，请刷新后重试" || text === "验证码已失效，请刷新后重试") return "验证码已失效，请点击“换一题”后重试。";
  if (text === "该邮箱域名不允许注册，请使用学校正式邮箱") return "该邮箱域名不允许注册，请使用学校正式邮箱。";
  if (text === "请先阅读并勾选同意《用户准则》后再提交") return "请先阅读并勾选同意《用户准则》后再提交。";
  if (text === "请求的资源不存在") return "注册接口不可用，请确认控制器服务已更新并重启。";
  return text;
}

function applyRegisterErrorToFields(msg: string) {
  const text = String(msg || "").trim();
  if (!text) return;
  if (text.startsWith("以下信息不可用：") || text.startsWith("以下信息已存在账号：")) {
    const fieldsRaw = text.replace(/^以下信息(不可用|已存在账号)：/, "").trim();
    for (const field of fieldsRaw.split(/[、，,]/g).map((item) => item.trim()).filter(Boolean)) {
      if (field === "用户名") fieldErrors.username = "该用户名已被占用，请更换后再提交。";
      if (field === "邮箱") fieldErrors.email = "该邮箱已被占用，请更换后再提交。";
      if (field === "学号") fieldErrors.student_id = "该学号已被占用，请确认后再提交。";
    }
    return;
  }
  if (text === "请完整填写注册信息") {
    validateRegisterFormLocal();
    return;
  }
  if (text === "请先阅读并勾选同意《用户准则》后再提交") {
    fieldErrors.accept_guideline = "请先阅读并勾选同意《用户准则》。";
    return;
  }
  if (text === "验证码错误，请重试" || text === "验证码已过期，请刷新后重试" || text === "验证码已失效，请刷新后重试") {
    fieldErrors.captcha = normalizeRegisterError(text);
    return;
  }
  if (text === "邮箱不能为空" || text === "邮箱格式不合法" || text.includes("注册邮箱后缀仅支持") || text.includes("邮箱前缀必须与学号一致")) {
    fieldErrors.email = normalizeFieldError("email", text);
    return;
  }
  if (text === "学号不能为空") {
    fieldErrors.student_id = "请填写学号。";
    return;
  }
  if (text === "用户名不能为空" || text.startsWith("用户名必须以学号 ") || text.includes("用户名必须写成“姓名缩写+学号”") || text === "用户名不得超过 18 个字符") {
    fieldErrors.username = normalizeFieldError("username", text);
    return;
  }
  if (text === "请先填写学号，再按“姓名缩写+学号”格式填写用户名") {
    fieldErrors.student_id = "请先填写学号。";
    fieldErrors.username = normalizeFieldError("username", text);
    return;
  }
  if (text.includes("强密码规则") || text === "密码不能包含空格") {
    fieldErrors.password = normalizeRegisterError(text);
    return;
  }
  if (text === "该邮箱请求过于频繁，请稍后再试") {
    fieldErrors.email = "该邮箱请求过于频繁，请等待倒计时结束后再试。";
  }
}

async function checkUnique(field?: FieldKey): Promise<boolean> {
  form.student_id = normalizeStudentIDInput(form.student_id);
  clearFieldError("username");
  clearFieldError("email");
  const usernameRaw = String(form.username || "").trim();
  if (usernameRaw) {
    fieldErrors.username = validateUsernameRuleLocal(usernameRaw, form.student_id);
    if (field === "username" && fieldErrors.username) return false;
  }
  const emailRaw = String(form.email || "").trim();
  if (emailRaw) {
    const normalized = normalizeRegisterEmailForStudent(emailRaw, form.student_id);
    form.email = normalized.value;
    if (normalized.error) {
      fieldErrors.email = normalized.error;
      if (field === "email" || field === "student_id") return false;
    }
  }
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const r = await client.authRegisterCheck({
      username: form.username,
      email: form.email,
      student_id: form.student_id,
    });
    fieldErrors.username = normalizeFieldError("username", r.errors?.username ?? "");
    if (!fieldErrors.email) {
      fieldErrors.email = normalizeFieldError("email", r.errors?.email ?? "");
    }
    fieldErrors.student_id = normalizeFieldError("student_id", r.errors?.student_id ?? "");
    if (field) return !(fieldErrors[field] ?? "");
    return !fieldErrors.username && !fieldErrors.email && !fieldErrors.student_id;
  } catch (e: any) {
    // 兼容旧后端：实时校验接口不存在时静默降级，避免点击背景触发 blur 后报错。
    if (e?.status === 404) {
      return true;
    }
    error.value = normalizeRegisterError(e?.message ?? String(e));
    return false;
  }
}

function validateRegisterFormLocal(): boolean {
  clearAllFieldErrors();
  form.student_id = normalizeStudentIDInput(form.student_id);
  const ym = parseYYYYMM(graduationYm.value);

  if (!String(form.real_name || "").trim()) {
    fieldErrors.real_name = "请填写真实姓名。";
  }
  if (!String(form.student_id || "").trim()) {
    fieldErrors.student_id = "请填写学号。";
  }
  fieldErrors.username = validateUsernameRuleLocal(form.username, form.student_id);
  const normalizedEmail = normalizeRegisterEmailForStudent(form.email, form.student_id);
  form.email = normalizedEmail.value;
  if (normalizedEmail.error) {
    fieldErrors.email = normalizedEmail.error;
  }
  if (!String(form.password || "").trim()) {
    fieldErrors.password = "请先设置密码。";
  } else {
    const pwdErr = checkStrongPassword(form.password);
    if (pwdErr) fieldErrors.password = pwdErr;
  }
  if (!String(confirmPassword.value || "").trim()) {
    fieldErrors.confirm_password = "请再次输入密码。";
  } else if (form.password !== confirmPassword.value) {
    fieldErrors.confirm_password = "两次密码输入不一致。";
  }
  if (!String(form.advisor || "").trim()) {
    fieldErrors.advisor = "请填写导师姓名。";
  }
  if (!ym) {
    fieldErrors.expected_graduation = "请选择合法的预计毕业年月。";
  }
  if (!String(form.phone || "").trim()) {
    fieldErrors.phone = "请填写联系电话。";
  }
  if (!acceptGuideline.value) {
    fieldErrors.accept_guideline = "请先阅读并勾选同意《用户准则》。";
  }
  if (!captchaId.value || captchaOption.value === null) {
    fieldErrors.captcha = "请先完成安全验证码。";
  }
  return !(Object.keys(fieldErrors) as FieldKey[]).some((k) => String(fieldErrors[k] || "").trim());
}

function stopCooldownTimer() {
  if (cooldownTimer) {
    clearInterval(cooldownTimer);
    cooldownTimer = null;
  }
}

function startRegisterCooldown(seconds: number, message: string) {
  stopCooldownTimer();
  cooldownRemainingSeconds.value = clampFrontendCooldownSeconds(seconds);
  cooldownMessage.value = String(message || "请求过于频繁，请稍后再试。");
  cooldownTimer = setInterval(() => {
    cooldownRemainingSeconds.value = Math.max(0, cooldownRemainingSeconds.value - 1);
    if (cooldownRemainingSeconds.value <= 0) {
      stopCooldownTimer();
      cooldownMessage.value = "";
    }
  }, 1000);
}

function parseRegisterErrorPayload(err: any): {
  error: string;
  retry_after_seconds?: number;
  field_errors?: Partial<Record<FieldKey, string>>;
} {
  const raw = String(err?.body || "").trim();
  if (!raw) return { error: String(err?.message ?? err ?? "").trim() };
  try {
    const parsed = JSON.parse(raw);
    return {
      error: String(parsed?.error ?? parsed?.message ?? err?.message ?? "").trim(),
      retry_after_seconds: Number(parsed?.retry_after_seconds ?? 0) || undefined,
      field_errors: parsed?.field_errors ?? undefined,
    };
  } catch {
    return { error: String(err?.message ?? err ?? "").trim() };
  }
}

async function submit() {
  if (submitted.value) return;
  error.value = "";
  success.value = "";
  if (registerLocked.value) {
    error.value = `当前触发安全冷却，请在 ${cooldownRemainingSeconds.value} 秒后再试。`;
    return;
  }
  if (!validateRegisterFormLocal()) {
    error.value = `请检查标红项：${firstFieldErrorMessage()}`;
    revealFirstInvalidField();
    return;
  }
  const ym = parseYYYYMM(graduationYm.value);
  if (!ym) {
    error.value = "请选择合法的预计毕业年月";
    return;
  }
  form.expected_graduation_year = ym.year;
  form.expected_graduation_month = ym.month;
  loading.value = true;
  try {
    const ok = await checkUnique();
    if (!ok) {
      error.value = "提交失败：用户名、学号或邮箱与现有账号/待审核申请冲突，请按下方提示逐项修改。";
      return;
    }
    const client = new ApiClient(settingsState.baseUrl);
    const r = await client.authRegister({
      ...form,
      accept_guideline: acceptGuideline.value,
      captcha_id: captchaId.value,
      captcha_option: Number(captchaOption.value),
    });
    success.value = r.message || t("验证邮件已发送，请前往邮箱点击链接完成提交。", "Verification email sent. Open the link in your mailbox to finish submission.");
    submitted.value = true;
    await ElMessageBox.alert(success.value, t("验证邮件已发送", "Verification Email Sent"), {
      type: "success",
      confirmButtonText: t("我知道了", "OK"),
    });
  } catch (e: any) {
    const payload = parseRegisterErrorPayload(e);
    if (payload.field_errors) {
      for (const [k, v] of Object.entries(payload.field_errors)) {
        const key = k as FieldKey;
        if (key in fieldErrors) {
          fieldErrors[key] = normalizeFieldError(key, String(v || ""));
        }
      }
    }
    applyRegisterErrorToFields(payload.error || e?.message || String(e));
    error.value = normalizeRegisterError(payload.error || e?.message || String(e));
    if (Number(payload.retry_after_seconds || 0) > 0) {
      const shownRetryAfter = clampFrontendCooldownSeconds(Number(payload.retry_after_seconds || 0));
      startRegisterCooldown(shownRetryAfter, `${error.value} ${shownRetryAfter} 秒后可再次尝试。`);
    }
    if ((Object.keys(fieldErrors) as FieldKey[]).some((k) => String(fieldErrors[k] || "").trim())) {
      revealFirstInvalidField();
    }
    await loadCaptcha();
  } finally {
    loading.value = false;
  }
}

async function loadCaptcha() {
  captchaLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const r = await client.authRegisterCaptcha();
    captchaId.value = String(r.captcha_id || "").trim();
    captchaQuestion.value = String(r.question || "").trim();
    captchaOptions.value = Array.isArray(r.options) ? r.options.map((v) => Number(v)) : [];
    captchaOption.value = null;
    clearFieldError("captcha");
  } catch (e: any) {
    captchaId.value = "";
    captchaQuestion.value = t("验证码加载失败，请稍后重试", "Captcha failed to load. Try again later.");
    captchaOptions.value = [];
    error.value = normalizeRegisterError(e?.message ?? String(e));
  } finally {
    captchaLoading.value = false;
  }
}

async function loadGuideline() {
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const r = await client.guideline();
    guidelineContent.value = String(r.content || "").trim() || t("暂无用户准则，请联系管理员。", "No guideline available yet. Contact the administrator.");
  } catch {
    guidelineContent.value = t("暂时无法加载用户准则，请稍后重试。", "Unable to load the user guidelines right now. Try again later.");
  }
}

async function handleVerifyResultFromQuery() {
  const status = String(route.query.email_verify || "").trim().toLowerCase();
  if (!status) return;
  const msg = String(route.query.verify_msg || "").trim();
  if (status === "ok") {
    success.value = msg || "邮箱验证成功，注册申请已提交，请耐心等待管理员审核。";
    submitted.value = true;
    verifiedSubmitted.value = true;
    await ElMessageBox.alert(success.value, t("提交成功", "Submitted"), {
      type: "success",
      confirmButtonText: t("我知道了", "OK"),
    });
  } else {
    error.value = msg || "邮箱验证失败，请返回注册页重新提交申请。";
    await ElMessageBox.alert(error.value, t("邮箱验证失败", "Email Verification Failed"), {
      type: "error",
      confirmButtonText: t("我知道了", "OK"),
    });
  }
  const nextQuery = { ...route.query } as Record<string, any>;
  delete nextQuery.email_verify;
  delete nextQuery.verify_msg;
  await router.replace({ path: route.path, query: nextQuery });
}

onMounted(() => {
  loadGuideline();
  loadCaptcha();
  handleVerifyResultFromQuery();
});

onBeforeUnmount(() => {
  stopCooldownTimer();
});
</script>

<style scoped>
.register-page {
  position: relative;
  min-height: 100vh;
  display: grid;
  place-items: center;
  padding: 20px;
  overflow: hidden;
  background:
    radial-gradient(1200px 600px at -10% -20%, rgba(249, 115, 22, 0.25), transparent 65%),
    radial-gradient(1000px 600px at 110% 120%, rgba(14, 165, 233, 0.22), transparent 65%),
    linear-gradient(160deg, #fef3c7 0%, #fff7ed 35%, #ecfeff 100%);
  background-size: 120% 120%, 130% 130%, 200% 200%;
  animation: gradientShift 12s ease-in-out infinite;
}
.bg-layer {
  position: absolute;
  inset: 0;
  pointer-events: none;
  overflow: hidden;
}
.bg-layer::before {
  content: "";
  position: absolute;
  inset: -20%;
  background: conic-gradient(from 0deg, rgba(251, 113, 133, 0.12), rgba(34, 211, 238, 0.1), rgba(245, 158, 11, 0.12), rgba(251, 113, 133, 0.12));
  animation: spinBg 24s linear infinite;
  filter: blur(20px);
}
.bg-layer::after {
  content: "";
  position: absolute;
  inset: -15%;
  background:
    repeating-linear-gradient(
      115deg,
      rgba(15, 118, 110, 0.08) 0 10px,
      rgba(255, 255, 255, 0) 10px 26px
    );
  animation: stream 16s linear infinite;
  opacity: 0.6;
}
.flow {
  position: absolute;
  border-radius: 999px;
  filter: blur(40px);
  opacity: 0.28;
}
.flow-a {
  width: 520px;
  height: 140px;
  left: -120px;
  top: 24%;
  transform: rotate(-18deg);
  background: linear-gradient(90deg, #0ea5e9, #14b8a6);
  animation: waveA 11s ease-in-out infinite;
}
.flow-b {
  width: 460px;
  height: 120px;
  right: -120px;
  bottom: 22%;
  transform: rotate(-22deg);
  background: linear-gradient(90deg, #f59e0b, #f97316);
  animation: waveB 13s ease-in-out infinite;
}
.blob {
  position: absolute;
  border-radius: 999px;
  filter: blur(30px);
  opacity: 0.45;
  animation: float 14s ease-in-out infinite;
}
.blob-a {
  width: 300px;
  height: 300px;
  left: -60px;
  top: 10%;
  background: #fb7185;
}
.blob-b {
  width: 380px;
  height: 380px;
  right: -80px;
  top: 35%;
  background: #22d3ee;
  animation-delay: 2s;
}
.blob-c {
  width: 260px;
  height: 260px;
  left: 45%;
  bottom: -80px;
  background: #f59e0b;
  animation-delay: 4s;
}
.spark {
  position: absolute;
  width: 24px;
  height: 24px;
  border-radius: 8px;
  transform: rotate(45deg);
  opacity: 0.6;
}
.spark-a {
  right: 20%;
  top: 16%;
  background: rgba(255, 255, 255, 0.62);
  animation: sparkle 4s ease-in-out infinite;
}
.spark-b {
  left: 18%;
  bottom: 18%;
  background: rgba(255, 255, 255, 0.5);
  animation: sparkle 5s ease-in-out infinite 0.6s;
}
.sticker {
  position: absolute;
  z-index: 1;
  padding: 8px 14px;
  border-radius: 999px;
  font-size: 13px;
  font-weight: 800;
  letter-spacing: 0.3px;
  color: #0f172a;
  background: rgba(255, 255, 255, 0.8);
  border: 2px solid rgba(255, 255, 255, 0.65);
  box-shadow: 0 10px 22px rgba(15, 23, 42, 0.14);
}
.sticker-a {
  right: 10%;
  top: 10%;
  transform: rotate(8deg);
}
.sticker-b {
  left: 8%;
  bottom: 8%;
  transform: rotate(-7deg);
}
.form-card {
  position: relative;
  z-index: 2;
  width: 100%;
  max-width: 1240px;
  border-radius: 20px;
  background: rgba(255, 255, 255, 0.9);
  backdrop-filter: blur(10px);
  overflow: hidden;
}
.lock-overlay {
  position: absolute;
  inset: 0;
  z-index: 20;
  display: grid;
  place-items: center;
  padding: 24px;
  background: rgba(15, 23, 42, 0.48);
  backdrop-filter: blur(10px);
  pointer-events: all;
}
.lock-panel {
  width: min(100%, 420px);
  border-radius: 24px;
  padding: 28px 24px;
  text-align: center;
  color: #e2e8f0;
  background:
    radial-gradient(circle at top, rgba(14, 165, 233, 0.2), transparent 55%),
    linear-gradient(180deg, rgba(15, 23, 42, 0.96), rgba(30, 41, 59, 0.94));
  box-shadow: 0 24px 80px rgba(15, 23, 42, 0.35);
  border: 1px solid rgba(148, 163, 184, 0.28);
}
.lock-title {
  font-size: 18px;
  font-weight: 800;
  color: #f8fafc;
}
.lock-count {
  margin-top: 12px;
  font-size: 64px;
  line-height: 1;
  font-weight: 900;
  color: #fbbf24;
  text-shadow: 0 8px 28px rgba(251, 191, 36, 0.35);
}
.lock-text {
  margin-top: 14px;
  font-size: 15px;
  line-height: 1.6;
  color: #e2e8f0;
}
.lock-sub {
  margin-top: 10px;
  font-size: 13px;
  color: #94a3b8;
}
.form-card :deep(.el-card__header) {
  padding: 22px 30px 8px;
}
.form-card :deep(.el-card__body) {
  padding: 10px 30px 28px;
}
.form-card :deep(.el-form-item__label) {
  font-size: 15px;
  font-weight: 700;
  color: #0f172a;
}
.form-card :deep(.el-input__wrapper) {
  min-height: 44px;
}
.form-card :deep(.el-input__inner) {
  font-size: 15px;
}
.form-card :deep(.el-alert__title) {
  font-size: 14px;
}
.field-item-invalid {
  animation: fieldShake 0.34s ease;
}
.field-item-invalid :deep(.el-input__wrapper),
.field-item-invalid :deep(.el-date-editor.el-input__wrapper),
.field-item-invalid :deep(.el-select__wrapper),
.field-item-invalid :deep(.el-radio-group),
.field-item-invalid .captcha-wrap {
  border-color: #dc2626 !important;
  box-shadow: 0 0 0 3px rgba(220, 38, 38, 0.12) !important;
}
.head h2 {
  margin: 0;
  font-size: 32px;
  line-height: 1.2;
  letter-spacing: 0.2px;
}
.head-top {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.head p {
  margin: 6px 0 0;
  color: #334155;
  font-size: 16px;
}
.mb {
  margin-bottom: 14px;
}
.rule-chips {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
  margin-bottom: 6px;
}
.rule-note {
  margin-bottom: 14px;
  font-size: 12px;
  color: #475569;
  font-weight: 600;
}
.chip {
  display: inline-flex;
  align-items: center;
  border-radius: 999px;
  padding: 4px 12px;
  font-size: 12px;
  font-weight: 700;
  border: 1px solid transparent;
}
.chip-orange {
  color: #9a3412;
  background: #ffedd5;
  border-color: #fdba74;
}
.chip-cyan {
  color: #155e75;
  background: #cffafe;
  border-color: #67e8f9;
}
.chip-blue {
  color: #1e3a8a;
  background: #dbeafe;
  border-color: #93c5fd;
}
.register-form {
  margin-bottom: 10px;
}
.section-grid {
  display: grid;
  grid-template-columns: repeat(2, minmax(0, 1fr));
  gap: 16px;
}
.form-section {
  border-radius: 16px;
  padding: 14px 14px 10px;
  border: 2px dashed rgba(148, 163, 184, 0.45);
  box-shadow: inset 0 1px 0 rgba(255, 255, 255, 0.6);
}
.section-account {
  background: linear-gradient(180deg, rgba(255, 247, 237, 0.95), rgba(255, 255, 255, 0.95));
}
.section-profile {
  background: linear-gradient(180deg, rgba(236, 254, 255, 0.95), rgba(255, 255, 255, 0.95));
}
.section-head {
  display: flex;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.section-dot {
  width: 14px;
  height: 14px;
  border-radius: 50%;
  flex: 0 0 auto;
  box-shadow: 0 0 0 4px rgba(255, 255, 255, 0.8);
}
.dot-account {
  background: #fb923c;
}
.dot-profile {
  background: #06b6d4;
}
.section-head h3 {
  margin: 0;
  font-size: 18px;
  color: #0f172a;
  line-height: 1.2;
}
.section-head p {
  margin: 2px 0 0;
  font-size: 12px;
  color: #475569;
}
.required {
  color: #dc2626;
  margin-right: 4px;
  font-weight: 700;
}
.field-tip {
  margin-top: 6px;
  font-size: 13px;
  color: #64748b;
}
.captcha-wrap {
  width: 100%;
  border: 1px dashed #94a3b8;
  border-radius: 10px;
  padding: 10px 12px;
  background: #ffffffc7;
}
.captcha-question {
  font-size: 14px;
  font-weight: 700;
  color: #0f172a;
  margin-bottom: 8px;
}
.captcha-options {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.captcha-actions {
  margin-top: 8px;
}
.field-error {
  margin-top: 6px;
  font-size: 13px;
  font-weight: 600;
  color: #dc2626;
  animation: fieldShake 0.34s ease;
}
.submit-btn {
  width: 100%;
  min-height: 46px;
  font-size: 16px;
  font-weight: 700;
}
.agree-line {
  margin-bottom: 12px;
  font-size: 14px;
  color: #334155;
}
.agree-line-invalid {
  animation: fieldShake 0.34s ease;
}
.guideline-link {
  border: none;
  background: transparent;
  color: #0c4a6e;
  padding: 0 2px;
  margin: 0;
  font-weight: 700;
  cursor: pointer;
  text-decoration: underline;
}
.guideline-wrap {
  max-height: 58vh;
  overflow: auto;
}
.guideline-wrap .md-body :deep(p) { margin: 8px 0; color:#334155; }
.guideline-wrap .md-body :deep(h1), .guideline-wrap .md-body :deep(h2), .guideline-wrap .md-body :deep(h3), .guideline-wrap .md-body :deep(h4) { margin: 10px 0; color: #0f172a; }
.guideline-wrap .md-body :deep(ul), .guideline-wrap .md-body :deep(ol) { padding-left: 20px; margin: 8px 0; color:#334155; }
.links {
  margin-top: 14px;
  font-size: 14px;
}
.links a {
  color: #0c4a6e;
  text-decoration: none;
  font-weight: 600;
}
@keyframes float {
  0%,
  100% {
    transform: translateY(0) translateX(0);
  }
  50% {
    transform: translateY(-18px) translateX(14px);
  }
}
@keyframes fieldShake {
  0%,
  100% {
    transform: translateX(0);
  }
  20% {
    transform: translateX(-6px);
  }
  40% {
    transform: translateX(5px);
  }
  60% {
    transform: translateX(-4px);
  }
  80% {
    transform: translateX(3px);
  }
}
@keyframes gradientShift {
  0%,
  100% {
    background-position: 0% 0%, 100% 100%, 0% 50%;
  }
  50% {
    background-position: 15% 10%, 85% 90%, 100% 50%;
  }
}
@keyframes spinBg {
  from {
    transform: rotate(0deg);
  }
  to {
    transform: rotate(360deg);
  }
}
@keyframes stream {
  from {
    transform: translateX(-6%) translateY(-4%);
  }
  to {
    transform: translateX(6%) translateY(4%);
  }
}
@keyframes waveA {
  0%,
  100% {
    transform: translateX(0) translateY(0) rotate(-18deg);
  }
  50% {
    transform: translateX(26px) translateY(-8px) rotate(-14deg);
  }
}
@keyframes waveB {
  0%,
  100% {
    transform: translateX(0) translateY(0) rotate(-22deg);
  }
  50% {
    transform: translateX(-22px) translateY(10px) rotate(-26deg);
  }
}
@keyframes sparkle {
  0%,
  100% {
    transform: rotate(45deg) scale(1);
    opacity: 0.55;
  }
  50% {
    transform: rotate(45deg) scale(1.24);
    opacity: 0.9;
  }
}
@media (max-width: 900px) {
  .form-card {
    max-width: 100%;
  }
  .form-card :deep(.el-card__header) {
    padding: 18px 16px 8px;
  }
  .form-card :deep(.el-card__body) {
    padding: 10px 16px 20px;
  }
  .head h2 {
    font-size: 26px;
  }
  .head p {
    font-size: 14px;
  }
  .section-grid {
    grid-template-columns: 1fr;
  }
  .form-section :deep(.el-col) {
    max-width: 100%;
    flex: 0 0 100%;
  }
  .sticker {
    font-size: 11px;
    padding: 6px 10px;
  }
  .lock-panel {
    width: 100%;
    padding: 24px 18px;
  }
  .lock-count {
    font-size: 52px;
  }
}
</style>
