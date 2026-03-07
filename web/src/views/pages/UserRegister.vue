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
      <template #header>
        <div class="head">
          <h2>平台账号注册申请</h2>
          <p>先完成邮箱验证，再进入管理员审核，通过后会进行邮箱通知(注意查询邮箱垃圾箱)。</p>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
      <el-alert
        v-if="verifiedSubmitted"
        title="请在审核通过后第一时间填写你已有的计算节点账号（节点账号页面），否则系统可能无法识别你的使用记录。"
        type="error"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-alert
        title="所有字段都必填。用户名、学号、邮箱不能和已注册账号、待审核申请或待验证申请重复。"
        type="warning"
        :closable="false"
        show-icon
        class="mb"
      />
      <div class="rule-chips">
        <span class="chip chip-orange">用户名：全平台唯一</span>
        <span class="chip chip-cyan">学号：全平台唯一</span>
        <span class="chip chip-blue">邮箱：仅允许 @example.org / @students.example.org</span>
      </div>
      <div class="rule-note">提交前会自动校验重复项；并强制校验“邮箱前缀=学号（学号自动转大写）”。</div>

      <el-form label-position="top" :disabled="submitted" class="register-form">
        <div class="section-grid">
          <section class="form-section section-account">
            <div class="section-head">
              <span class="section-dot dot-account" />
              <div>
                <h3>账号信息</h3>
                <p>用于登录平台，注意唯一性。</p>
              </div>
            </div>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 邮箱</template>
                  <el-input v-model="form.email" placeholder="例如：26B123456@example.org" @blur="checkUnique('email')" />
                  <div class="field-tip">必须使用 `@example.org` 或 `@students.example.org`；邮箱前缀必须与学号一致。</div>
                  <div v-if="fieldErrors.email" class="field-error">{{ fieldErrors.email }}</div>
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 用户名</template>
                  <el-input v-model="form.username" placeholder="例如：zs22B123456（不超过18字符）" @blur="checkUnique('username')" />
                  <div class="field-tip">建议使用姓名缩写+学号，例如 zs22B123456，最多 18 个字符。</div>
                  <div v-if="fieldErrors.username" class="field-error">{{ fieldErrors.username }}</div>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 密码</template>
                  <el-input v-model="form.password" type="password" show-password placeholder="请设置强密码" />
                  <div class="field-tip">{{ passwordRuleText }}</div>
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 确认密码</template>
                  <el-input v-model="confirmPassword" type="password" show-password />
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item required>
              <template #label><span class="required">*</span> 安全验证码（每次注册必做）</template>
              <div class="captcha-wrap">
                <div class="captcha-question">{{ captchaQuestionLabel }}</div>
                <el-radio-group v-model="captchaOption" class="captcha-options">
                  <el-radio-button v-for="(op, idx) in captchaOptions" :key="`${captchaId}-${idx}-${op}`" :label="idx">{{ op }}</el-radio-button>
                </el-radio-group>
                <div class="captcha-actions">
                  <el-button text type="primary" :loading="captchaLoading" @click="loadCaptcha">换一题</el-button>
                </div>
              </div>
            </el-form-item>
          </section>

          <section class="form-section section-profile">
            <div class="section-head">
              <span class="section-dot dot-profile" />
              <div>
                <h3>身份信息</h3>
                <p>请填写真实资料，便于管理员审核。</p>
              </div>
            </div>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 真实姓名</template>
                  <el-input v-model="form.real_name" placeholder="请填写真实中文姓名，例如：张三" />
                  <div class="field-tip">请使用真实姓名的汉字形式。</div>
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 学号</template>
                  <el-input v-model="form.student_id" placeholder="注意全大写，例如26B123456" @input="onStudentInput" @blur="checkUnique('student_id')" />
                  <div class="field-tip">输入小写会自动转为大写；并将用于校验邮箱前缀。</div>
                  <div v-if="fieldErrors.student_id" class="field-error">{{ fieldErrors.student_id }}</div>
                </el-form-item>
              </el-col>
            </el-row>
            <el-row :gutter="18">
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 导师</template>
                  <el-input v-model="form.advisor" />
                </el-form-item>
              </el-col>
              <el-col :span="24">
                <el-form-item required>
                  <template #label><span class="required">*</span> 预计毕业时间（年-月）</template>
                  <el-date-picker
                    v-model="graduationYm"
                    type="month"
                    value-format="YYYY-MM"
                    format="YYYY-MM"
                    style="width: 100%"
                    placeholder="请选择毕业年月"
                  />
                </el-form-item>
              </el-col>
            </el-row>
            <el-form-item required>
              <template #label><span class="required">*</span> 电话</template>
              <el-input v-model="form.phone" />
            </el-form-item>
          </section>
        </div>
      </el-form>

      <div class="agree-line">
        <el-checkbox v-model="acceptGuideline" :disabled="submitted">
          我已阅读并同意
          <button type="button" class="guideline-link" @click.prevent="guidelineVisible = true">《用户准则》</button>
          ，自觉遵守平台规范，否则后果自负。
        </el-checkbox>
      </div>

      <el-button
        :type="submitted ? 'info' : 'primary'"
        :loading="loading"
        :disabled="submitted"
        @click="submit"
        class="submit-btn"
      >
        {{ verifiedSubmitted ? "已提交，等待审核" : submitted ? "验证邮件已发送" : "提交注册申请" }}
      </el-button>

      <div class="links">
        <router-link to="/login">返回登录</router-link>
      </div>
    </el-card>

    <el-dialog v-model="guidelineVisible" title="用户准则" width="760px">
      <div class="guideline-wrap">
        <div class="md-body" v-html="renderMarkdown(guidelineContent)" />
      </div>
      <template #footer>
        <el-button @click="guidelineVisible = false">关闭</el-button>
      </template>
    </el-dialog>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { ElMessageBox } from "element-plus";
import { renderMarkdown } from "../../lib/markdown";
import { STRONG_PASSWORD_RULE_TEXT, checkStrongPassword } from "../../lib/passwordPolicy";
import { getServerCurrentYear } from "../../lib/time";

type FieldKey = "username" | "email" | "student_id";
const allowedEmailDomains = ["example.org", "students.example.org"];

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
const captchaQuestionLabel = computed(() => captchaQuestion.value || "验证码加载中...");
const fieldErrors = reactive<Record<FieldKey, string>>({
  username: "",
  email: "",
  student_id: "",
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

function normalizeStudentIDInput(v: string): string {
  return String(v || "").trim().toUpperCase();
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

function onStudentInput() {
  form.student_id = normalizeStudentIDInput(form.student_id);
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
  if (text === "用户名不得超过 18 个字符") return "用户名最多 18 个字符，请缩短后再提交。";
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

async function checkUnique(field?: FieldKey): Promise<boolean> {
  form.student_id = normalizeStudentIDInput(form.student_id);
  fieldErrors.email = "";
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

async function submit() {
  if (submitted.value) return;
  error.value = "";
  success.value = "";
  form.student_id = normalizeStudentIDInput(form.student_id);
  const ym = parseYYYYMM(graduationYm.value);
  if (!ym) {
    error.value = "请选择合法的预计毕业年月";
    return;
  }
  form.expected_graduation_year = ym.year;
  form.expected_graduation_month = ym.month;
  if ((form.username || "").trim().length > 18) {
    error.value = "用户名不得超过 18 个字符";
    return;
  }
  const normalizedEmail = normalizeRegisterEmailForStudent(form.email, form.student_id);
  form.email = normalizedEmail.value;
  if (normalizedEmail.error) {
    error.value = normalizedEmail.error;
    return;
  }
  if (form.password !== confirmPassword.value) {
    error.value = "两次密码输入不一致";
    return;
  }
  const pwdErr = checkStrongPassword(form.password);
  if (pwdErr) {
    error.value = pwdErr;
    return;
  }
  if (!acceptGuideline.value) {
    error.value = "请先阅读并勾选同意《用户准则》";
    return;
  }
  if (!captchaId.value || captchaOption.value === null) {
    error.value = "请先完成安全验证码";
    return;
  }
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
    success.value = r.message || "验证邮件已发送，请前往邮箱点击链接完成提交。";
    submitted.value = true;
    await ElMessageBox.alert(success.value, "验证邮件已发送", {
      type: "success",
      confirmButtonText: "我知道了",
    });
  } catch (e: any) {
    error.value = normalizeRegisterError(e?.message ?? String(e));
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
  } catch (e: any) {
    captchaId.value = "";
    captchaQuestion.value = "验证码加载失败，请稍后重试";
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
    guidelineContent.value = String(r.content || "").trim() || "暂无用户准则，请联系管理员。";
  } catch {
    guidelineContent.value = "暂时无法加载用户准则，请稍后重试。";
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
    await ElMessageBox.alert(success.value, "提交成功", {
      type: "success",
      confirmButtonText: "我知道了",
    });
  } else {
    error.value = msg || "邮箱验证失败，请返回注册页重新提交申请。";
    await ElMessageBox.alert(error.value, "邮箱验证失败", {
      type: "error",
      confirmButtonText: "我知道了",
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
.head h2 {
  margin: 0;
  font-size: 32px;
  line-height: 1.2;
  letter-spacing: 0.2px;
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
}
</style>
