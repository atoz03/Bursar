<template>
  <el-card>
    <template #header>
      <div class="row">
        <div>
          <div class="title">我的积分</div>
          <div class="sub">登录态自动识别账号</div>
        </div>
        <el-space>
          <el-button @click="goProfile">个人信息修改</el-button>
          <el-button :loading="loading" type="primary" @click="query">刷新</el-button>
        </el-space>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon />
    <el-card v-if="announcements.length > 0" style="margin-bottom: 12px">
      <template #header><b>公告</b></template>
      <div v-for="a in announcements" :key="a.announcement_id" style="padding: 6px 0; border-bottom: 1px solid #eef2f7">
        <div style="font-weight: 600">{{ a.pinned ? "📌 " : "" }}{{ a.title }}</div>
        <div style="color:#475569; white-space: pre-wrap">{{ a.content }}</div>
      </div>
    </el-card>

    <el-descriptions v-if="resp" :column="2" border>
      <el-descriptions-item label="用户名">{{ resp.username }}</el-descriptions-item>
      <el-descriptions-item label="积分">{{ resp.balance }}</el-descriptions-item>
      <el-descriptions-item label="状态">
        <el-tag :type="tagType(resp.status)">{{ resp.status }}</el-tag>
      </el-descriptions-item>
    </el-descriptions>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ApiClient, type Announcement, type BalanceResp } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { useRouter } from "vue-router";

const loading = ref(false);
const error = ref("");
const resp = ref<BalanceResp | null>(null);
const announcements = ref<Announcement[]>([]);
const router = useRouter();

function tagType(status: string) {
  if (status === "normal") return "success";
  if (status === "warning") return "warning";
  if (status === "limited") return "danger";
  if (status === "blocked") return "danger";
  return "info";
}

async function query() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl);
    resp.value = await client.userMyBalance();
    const ar = await client.announcements(10);
    announcements.value = ar.announcements ?? [];
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    loading.value = false;
  }
}

function goProfile() {
  router.push("/user/profile");
}

query();
</script>

<style scoped>
.row { display: flex; align-items: center; justify-content: space-between; gap: 12px; }
.title { font-weight: 700; }
.sub { margin-top: 4px; font-size: 12px; color: #64748b; }
</style>
