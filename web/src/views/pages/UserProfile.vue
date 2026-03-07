<template>
  <div class="user-fun-page">
    <div class="user-fun-bg">
      <div class="user-fun-flow a" />
      <div class="user-fun-flow b" />
      <div class="user-fun-blob a" />
      <div class="user-fun-blob b" />
      <div class="user-fun-spark a" />
      <div class="user-fun-spark b" />
      <div class="user-fun-sticker left">真实信息</div>
      <div class="user-fun-sticker right">审核更顺畅</div>
    </div>
    <el-card class="user-fun-card profile-card">
      <template #header>
        <div class="row">
          <div>
            <h2 class="user-fun-head-title">个人信息</h2>
            <p class="user-fun-head-sub">可直接修改基础信息；用户名/邮箱/学号变更需管理员审核</p>
          </div>
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </template>

      <el-alert
        title="提示：修改“用户名、邮箱、学号”时，必须填写备注说明，提交后进入管理员审核。"
        type="warning"
        show-icon
        class="mb"
      />
      <el-alert
        title="唯一性规则：用户名、邮箱、学号这三项必须全平台唯一，不能与已开通账号或待审核申请重复。"
        type="info"
        show-icon
        :closable="false"
        class="mb"
      />
      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

      <el-form label-position="top">
        <el-row :gutter="12">
          <el-col :span="12"><el-form-item label="邮箱 *" required><el-input v-model="form.email" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="用户名 *" required><el-input v-model="form.username" /></el-form-item></el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12"><el-form-item label="真实姓名 *" required><el-input v-model="form.real_name" /></el-form-item></el-col>
          <el-col :span="12"><el-form-item label="学号 *" required><el-input v-model="form.student_id" /></el-form-item></el-col>
        </el-row>
        <el-row :gutter="12">
          <el-col :span="12"><el-form-item label="导师 *" required><el-input v-model="form.advisor" /></el-form-item></el-col>
          <el-col :span="12">
            <el-form-item label="预计毕业时间（年-月） *" required>
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
        <el-form-item label="电话 *" required><el-input v-model="form.phone" /></el-form-item>
        <el-form-item label="变更备注（仅修改用户名/邮箱/学号时必填）">
          <el-input v-model="form.change_reason" type="textarea" :rows="3" placeholder="请说明变更原因，供管理员审核" />
        </el-form-item>
        <el-form-item>
          <el-button type="primary" :loading="saving" @click="save">保存</el-button>
        </el-form-item>
      </el-form>

      <el-divider class="user-fun-divider" />
      <h3 class="record-title">关键信息变更申请记录</h3>
      <el-table :data="requests" stripe>
        <el-table-column prop="request_id" label="ID" width="90" />
        <el-table-column prop="status" label="状态" width="120" />
        <el-table-column prop="old_username" label="原用户名" width="130" />
        <el-table-column prop="new_username" label="新用户名" width="130" />
        <el-table-column prop="old_email" label="原邮箱" min-width="180" />
        <el-table-column prop="new_email" label="新邮箱" min-width="180" />
        <el-table-column prop="old_student_id" label="原学号" width="130" />
        <el-table-column prop="new_student_id" label="新学号" width="130" />
        <el-table-column prop="reason" label="备注" min-width="200" />
        <el-table-column prop="created_at" label="提交时间" width="180" :formatter="tableTimeFormatter" />
      </el-table>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { reactive, ref } from "vue";
import { ApiClient, type ProfileChangeRequest } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime } from "../../lib/time";

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
const saving = ref(false);
const error = ref("");
const success = ref("");
const requests = ref<ProfileChangeRequest[]>([]);
const graduationYm = ref(toYYYYMM(new Date().getFullYear() + 3, 6));

const form = reactive({
  email: "",
  username: "",
  real_name: "",
  student_id: "",
  advisor: "",
  expected_graduation_year: new Date().getFullYear() + 3,
  expected_graduation_month: 6,
  phone: "",
  change_reason: "",
});

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const [me, reqs] = await Promise.all([client().userMe(), client().userProfileChangeRequests(100)]);
    form.email = me.email ?? "";
    form.username = me.username ?? "";
    form.real_name = me.real_name ?? "";
    form.student_id = me.student_id ?? "";
    form.advisor = me.advisor ?? "";
    form.expected_graduation_year = me.expected_graduation_year ?? new Date().getFullYear() + 3;
    form.expected_graduation_month = me.expected_graduation_month ?? 6;
    graduationYm.value = toYYYYMM(form.expected_graduation_year, form.expected_graduation_month);
    form.phone = me.phone ?? "";
    requests.value = reqs.requests ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function save() {
  saving.value = true;
  error.value = "";
  success.value = "";
  const ym = parseYYYYMM(graduationYm.value);
  if (!ym) {
    error.value = "请选择合法的预计毕业年月";
    saving.value = false;
    return;
  }
  form.expected_graduation_year = ym.year;
  form.expected_graduation_month = ym.month;
  try {
    const r = await client().userUpdateProfile({ ...form });
    success.value = r.message || "保存成功";
    form.change_reason = "";
    await reload();
  } catch (e: any) {
    const msg = String(e?.message ?? e ?? "").trim();
    if (msg === "请求的资源不存在") {
      error.value = "资料保存接口不可用，请确认控制器已更新并重启。";
    } else {
      error.value = msg || "保存失败，请稍后重试";
    }
  } finally {
    saving.value = false;
  }
}

reload();
</script>

<style scoped>
.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.profile-card { min-height: 620px; }
.record-title {
  margin: 0 0 12px;
  font-size: 18px;
  color: #0f172a;
}
.mb { margin-bottom: 12px; }

@media (max-width: 900px) {
  .row { flex-wrap: wrap; }
}
</style>
