<template>
  <div class="setup-page">
    <div class="setup-head">
      <div>
        <div class="eyebrow">FIRST-RUN SETUP</div>
        <h2>{{ form.platform.platform_name || "Bursar" }} 系统设置</h2>
        <p>集中完成平台身份、注册规则、计费、邮件和容灾配置。启动密钥只做状态检查，不会回传到浏览器。</p>
      </div>
      <el-tag :type="form.platform.setup_completed ? 'success' : 'warning'">
        {{ form.platform.setup_completed ? "已完成" : "待完成" }}
      </el-tag>
    </div>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert
      v-if="!form.platform.setup_completed"
      title="完成设置前，管理员会被固定引导到本页，公开注册入口也会保持关闭。"
      type="warning"
      :closable="false"
      show-icon
      class="mb"
    />

    <el-card v-loading="loading" class="setup-card">
      <el-steps :active="step" finish-status="success" align-center class="steps">
        <el-step title="平台与注册" />
        <el-step title="计费与准则" />
        <el-step title="邮件通知" />
        <el-step title="容灾与确认" />
      </el-steps>

      <section v-show="step === 0" class="step-panel">
        <div class="section-title">
          <h3>平台与注册</h3>
          <p>这些内容会替代旧项目中的机构名称、固定邮箱和 SSH 域名。</p>
        </div>
        <el-form label-position="top" :model="form.platform">
          <el-form-item label="平台名称" required>
            <el-input v-model="form.platform.platform_name" maxlength="80" show-word-limit placeholder="例如：Bursar" />
          </el-form-item>
          <el-form-item label="允许注册的邮箱域名">
            <el-select
              v-model="form.platform.registration_allowed_email_domains"
              multiple
              filterable
              allow-create
              default-first-option
              style="width: 100%"
              placeholder="留空表示允许任意有效邮箱；输入 example.org 后回车"
            />
            <div class="hint">只填域名，不要带 @。适合学校或公司内部部署；公开部署可留空。</div>
          </el-form-item>
          <el-form-item label="统一 SSH 入口">
            <el-input v-model="form.platform.provision_ssh_host" placeholder="例如：gpu.example.org；没有统一入口可留空" />
            <div class="hint">用于管理员开通节点账号时预填 SSH 主机，不包含协议、用户名和端口。</div>
          </el-form-item>
        </el-form>
      </section>

      <section v-show="step === 1" class="step-panel">
        <div class="section-title">
          <h3>资源价格与用户准则</h3>
          <p>价格单位沿用系统现有定义：每分钟扣减积分。节点也可在接入后单独覆盖价格。</p>
        </div>
        <el-table :data="form.prices" border size="small" class="price-table">
          <el-table-column label="资源型号" min-width="220">
            <template #default="scope">
              <el-input v-model="scope.row.Model" placeholder="例如：NVIDIA RTX 4090" />
            </template>
          </el-table-column>
          <el-table-column label="每分钟价格" min-width="180">
            <template #default="scope">
              <el-input-number v-model="scope.row.Price" :min="0" :precision="4" :step="0.01" style="width: 100%" />
            </template>
          </el-table-column>
          <el-table-column width="86" align="center">
            <template #default="scope">
              <el-button text type="danger" @click="form.prices.splice(scope.$index, 1)">删除</el-button>
            </template>
          </el-table-column>
        </el-table>
        <el-button class="add-price" @click="form.prices.push({ Model: '', Price: 0 })">新增价格</el-button>
        <el-form-item label="用户准则" required class="guideline-field">
          <el-input v-model="form.user_guideline" type="textarea" :rows="12" placeholder="使用 Markdown 编写用户必须同意的规则" />
        </el-form-item>
      </section>

      <section v-show="step === 2" class="step-panel">
        <div class="section-title with-switch">
          <div>
            <h3>邮件通知</h3>
            <p>注册验证、密码找回和管理员通知依赖 SMTP；暂不使用时可以关闭并跳过。</p>
          </div>
          <el-switch v-model="form.mail.enabled" active-text="启用" inactive-text="关闭" />
        </div>
        <el-form label-position="top" :disabled="!form.mail.enabled">
          <el-row :gutter="18">
            <el-col :xs="24" :md="16"><el-form-item label="SMTP 主机" required><el-input v-model="form.mail.smtp_host" placeholder="smtp.example.org" /></el-form-item></el-col>
            <el-col :xs="24" :md="8"><el-form-item label="端口" required><el-input-number v-model="form.mail.smtp_port" :min="1" :max="65535" style="width: 100%" /></el-form-item></el-col>
          </el-row>
          <el-row :gutter="18">
            <el-col :xs="24" :md="12"><el-form-item label="SMTP 用户名" required><el-input v-model="form.mail.smtp_user" /></el-form-item></el-col>
            <el-col :xs="24" :md="12">
              <el-form-item :label="form.mail.smtp_password_set ? 'SMTP 密码（留空保持不变）' : 'SMTP 密码'" required>
                <el-input v-model="form.mail.smtp_pass" type="password" show-password autocomplete="new-password" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="18">
            <el-col :xs="24" :md="12"><el-form-item label="发件邮箱" required><el-input v-model="form.mail.from_email" /></el-form-item></el-col>
            <el-col :xs="24" :md="12"><el-form-item label="发件人名称" required><el-input v-model="form.mail.from_name" /></el-form-item></el-col>
          </el-row>
        </el-form>
      </section>

      <section v-show="step === 3" class="step-panel">
        <div class="section-title with-switch">
          <div>
            <h3>容灾同步</h3>
            <p>单控制器部署请保持关闭；主备部署时再填写真实的对端连接信息。</p>
          </div>
          <el-switch v-model="form.ha.enabled" active-text="启用" inactive-text="关闭" />
        </div>
        <el-form label-position="top" :disabled="!form.ha.enabled" class="ha-form">
          <el-row :gutter="18">
            <el-col :xs="24" :md="8"><el-form-item label="容灾节点 ID" required><el-input v-model="form.ha.dr_node_id" placeholder="standby-1" /></el-form-item></el-col>
            <el-col :xs="24" :md="8"><el-form-item label="容灾主机" required><el-input v-model="form.ha.dr_host" placeholder="192.0.2.20" /></el-form-item></el-col>
            <el-col :xs="24" :md="8"><el-form-item label="SSH 用户" required><el-input v-model="form.ha.dr_ssh_user" placeholder="gpuops" /></el-form-item></el-col>
          </el-row>
          <el-row :gutter="18">
            <el-col :xs="24" :md="8"><el-form-item label="SSH 端口"><el-input-number v-model="form.ha.dr_ssh_port" :min="1" :max="65535" style="width: 100%" /></el-form-item></el-col>
            <el-col :xs="24" :md="8"><el-form-item label="对端控制器端口"><el-input-number v-model="form.ha.dr_controller_port" :min="1" :max="65535" style="width: 100%" /></el-form-item></el-col>
            <el-col :xs="24" :md="8"><el-form-item label="同步小时"><el-input-number v-model="form.ha.start_hour" :min="0" :max="23" style="width: 100%" /></el-form-item></el-col>
          </el-row>
          <el-form-item label="容灾 SSH 私钥路径" required><el-input v-model="form.ha.dr_key_file" placeholder="/etc/gpu-ops/standby_ed25519" /></el-form-item>
          <el-form-item label="同步脚本路径" required><el-input v-model="form.ha.script_path" placeholder="/opt/gpu-ops/scripts/ha_sync_worker.sh" /></el-form-item>
        </el-form>

        <el-divider>启动配置检查</el-divider>
        <div class="check-grid">
          <div v-for="check in form.checks" :key="check.key" class="check-item" :class="check.ok ? 'ok' : 'bad'">
            <el-icon><CircleCheck v-if="check.ok" /><WarningFilled v-else /></el-icon>
            <div><b>{{ check.label }}</b><p>{{ check.ok ? "已就绪" : check.message }}</p></div>
            <el-tag v-if="check.required" size="small" :type="check.ok ? 'success' : 'danger'">必填</el-tag>
          </div>
        </div>
        <el-descriptions :column="1" border size="small" class="startup-info">
          <el-descriptions-item label="Web 监听">{{ form.startup.listen_addr || "-" }}</el-descriptions-item>
          <el-descriptions-item label="内部监听">{{ form.startup.internal_listen_addr || "未启用" }}</el-descriptions-item>
          <el-descriptions-item label="节点共享目录">{{ form.startup.shared_node_root || "-" }}</el-descriptions-item>
          <el-descriptions-item label="集群共享目录">{{ form.startup.shared_cluster_root || "-" }}</el-descriptions-item>
        </el-descriptions>
      </section>

      <div class="actions">
        <el-button :disabled="step === 0" @click="step -= 1">上一步</el-button>
        <el-button v-if="step < 3" type="primary" @click="nextStep">下一步</el-button>
        <el-button v-else type="primary" :loading="saving" :disabled="hasRequiredCheckFailure" @click="save">保存并完成设置</el-button>
      </div>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { computed, onMounted, reactive, ref } from "vue";
import { useRouter } from "vue-router";
import { CircleCheck, WarningFilled } from "@element-plus/icons-vue";
import { ElMessage } from "element-plus";
import { ApiClient, type AdminSetupState, type HASyncConfig, type PlatformSettings, type SetupMailSettings, type SetupStartupCheck } from "../../lib/api";
import { authState, refreshAuth } from "../../lib/authStore";
import { settingsState } from "../../lib/settingsStore";

type PriceDraft = { Model: string; Price: number };

const router = useRouter();
const loading = ref(false);
const saving = ref(false);
const error = ref("");
const step = ref(0);

const form = reactive({
  platform: {
    platform_name: "Bursar",
    registration_allowed_email_domains: [] as string[],
    provision_ssh_host: "",
    setup_completed: false,
  } as PlatformSettings,
  user_guideline: "",
  prices: [] as PriceDraft[],
  mail: {
    enabled: false,
    smtp_host: "",
    smtp_port: 587,
    smtp_user: "",
    smtp_pass: "",
    smtp_password_set: false,
    from_email: "",
    from_name: "Bursar",
  } as SetupMailSettings,
  ha: {
    enabled: false,
    interval_days: 1,
    start_hour: 3,
    dr_node_id: "",
    dr_host: "",
    dr_ssh_port: 22,
    dr_ssh_user: "",
    dr_key_file: "",
    dr_controller_port: 8080,
    primary_host: "127.0.0.1",
    primary_controller_port: 8080,
    script_path: "/opt/gpu-ops/scripts/ha_sync_worker.sh",
    sync_web_dist: true,
    sync_database: true,
    auto_failover: false,
  } as HASyncConfig,
  startup: { listen_addr: "", internal_listen_addr: "", shared_node_root: "", shared_cluster_root: "" },
  checks: [] as SetupStartupCheck[],
});

const hasRequiredCheckFailure = computed(() => form.checks.some((check) => check.required && !check.ok));

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function applyState(state: AdminSetupState) {
  Object.assign(form.platform, state.platform || {});
  form.platform.registration_allowed_email_domains = Array.isArray(state.platform?.registration_allowed_email_domains)
    ? [...state.platform.registration_allowed_email_domains]
    : [];
  form.user_guideline = String(state.user_guideline || "");
  form.prices = (state.prices || []).map((row) => ({
    Model: String(row.Model ?? row.model ?? ""),
    Price: Number(row.Price ?? row.price ?? 0),
  }));
  Object.assign(form.mail, state.mail || {});
  form.mail.smtp_pass = "";
  Object.assign(form.ha, state.ha || {});
  Object.assign(form.startup, state.startup || {});
  form.checks = Array.isArray(state.checks) ? [...state.checks] : [];
}

async function load() {
  loading.value = true;
  error.value = "";
  try {
    applyState(await client().adminSetup());
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function validateCurrentStep(): string {
  if (step.value === 0) {
    if (!form.platform.platform_name.trim()) return "请填写平台名称";
    const invalidDomain = form.platform.registration_allowed_email_domains.find((domain) => !/^[a-z0-9.-]+\.[a-z]{2,}$/i.test(String(domain || "").replace(/^@/, "")));
    if (invalidDomain) return `邮箱域名格式不合法：${invalidDomain}`;
  }
  if (step.value === 1) {
    if (!form.user_guideline.trim()) return "请填写用户准则";
    if (form.prices.some((row) => !row.Model.trim() || !Number.isFinite(row.Price) || row.Price < 0)) return "请完整填写资源型号和非负价格";
  }
  if (step.value === 2 && form.mail.enabled) {
    if (!form.mail.smtp_host.trim() || !form.mail.smtp_user.trim() || !form.mail.from_email.trim() || !form.mail.from_name.trim()) return "请完整填写 SMTP 与发件人信息";
    if (!form.mail.smtp_password_set && !String(form.mail.smtp_pass || "").trim()) return "首次启用邮件通知时必须填写 SMTP 密码";
  }
  if (step.value === 3 && form.ha.enabled) {
    if (!form.ha.dr_node_id.trim() || !form.ha.dr_host.trim() || !form.ha.dr_ssh_user.trim() || !form.ha.dr_key_file.trim() || !form.ha.script_path.trim()) return "请完整填写容灾连接信息";
  }
  return "";
}

function nextStep() {
  const message = validateCurrentStep();
  if (message) {
    ElMessage.warning(message);
    return;
  }
  step.value = Math.min(3, step.value + 1);
}

async function save() {
  const message = validateCurrentStep();
  if (message) {
    ElMessage.warning(message);
    return;
  }
  if (hasRequiredCheckFailure.value) {
    ElMessage.error("请先修复启动配置中的必填安全项并重启控制器");
    return;
  }
  saving.value = true;
  error.value = "";
  try {
    form.platform.registration_allowed_email_domains = form.platform.registration_allowed_email_domains
      .map((domain) => String(domain || "").trim().replace(/^@/, "").toLowerCase())
      .filter(Boolean);
    const response = await client().adminSaveSetup({
      platform: { ...form.platform },
      user_guideline: form.user_guideline,
      prices: form.prices.map((row) => ({ Model: row.Model.trim(), Price: Number(row.Price) })),
      mail: { ...form.mail },
      ha: { ...form.ha },
    });
    applyState(response.setup);
    await refreshAuth();
    ElMessage.success("系统设置已保存");
    await router.push("/admin/board");
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    saving.value = false;
  }
}

onMounted(load);
</script>

<style scoped>
.setup-page { max-width: 1120px; margin: 0 auto; padding: 24px; }
.setup-head { display: flex; align-items: flex-start; justify-content: space-between; gap: 20px; margin-bottom: 18px; }
.setup-head h2 { margin: 5px 0 8px; font-size: 30px; color: #0f172a; }
.setup-head p, .section-title p { margin: 0; color: #64748b; line-height: 1.7; }
.eyebrow { color: #2563eb; font-size: 12px; font-weight: 800; letter-spacing: .14em; }
.mb { margin-bottom: 16px; }
.setup-card { border-radius: 18px; }
.steps { margin: 8px 0 32px; }
.step-panel { min-height: 430px; padding: 4px 28px 24px; }
.section-title { margin-bottom: 22px; }
.section-title h3 { margin: 0 0 6px; font-size: 22px; color: #172033; }
.with-switch { display: flex; justify-content: space-between; align-items: flex-start; gap: 18px; }
.hint { margin-top: 7px; color: #7c8aa0; font-size: 13px; }
.price-table { margin-bottom: 12px; }
.add-price { margin-bottom: 24px; }
.guideline-field { margin-top: 6px; }
.check-grid { display: grid; grid-template-columns: repeat(2, minmax(0, 1fr)); gap: 12px; margin-bottom: 18px; }
.check-item { display: flex; align-items: flex-start; gap: 10px; padding: 13px 14px; border: 1px solid #dbe4ef; border-radius: 12px; background: #f8fafc; }
.check-item.ok { border-color: #bbf7d0; background: #f0fdf4; }
.check-item.bad { border-color: #fecaca; background: #fff7f7; }
.check-item .el-icon { margin-top: 2px; font-size: 20px; }
.check-item.ok .el-icon { color: #16a34a; }
.check-item.bad .el-icon { color: #dc2626; }
.check-item div { flex: 1; }
.check-item b { color: #1e293b; }
.check-item p { margin: 4px 0 0; color: #64748b; font-size: 12px; line-height: 1.5; }
.startup-info { margin-top: 12px; }
.actions { display: flex; justify-content: flex-end; gap: 10px; padding: 20px 28px 8px; border-top: 1px solid #e5e7eb; }
@media (max-width: 760px) {
  .setup-page { padding: 14px; }
  .step-panel { padding: 0 2px 18px; }
  .check-grid { grid-template-columns: 1fr; }
  .setup-head { display: block; }
  .setup-head .el-tag { margin-top: 10px; }
}
</style>
