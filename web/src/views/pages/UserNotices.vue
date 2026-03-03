<template>
  <div class="user-fun-page">
    <div class="user-fun-bg">
      <div class="user-fun-flow a" />
      <div class="user-fun-flow b" />
      <div class="user-fun-blob a" />
      <div class="user-fun-blob b" />
      <div class="user-fun-spark a" />
      <div class="user-fun-spark b" />
      <div class="user-fun-sticker left">最新通知</div>
      <div class="user-fun-sticker right">准则回顾</div>
    </div>
    <el-card class="user-fun-card notice-card">
      <template #header>
        <div class="row">
          <div>
            <h2 class="user-fun-head-title">公告与用户准则</h2>
            <p class="user-fun-head-sub">有新公告会在左侧菜单标记提醒，这里可集中查看</p>
          </div>
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />

      <el-card class="mb">
        <template #header><b>用户准则</b></template>
        <div class="md-body" v-html="renderMarkdown(guidelineContent)" />
      </el-card>

      <el-card>
        <template #header>
          <div class="row">
            <b>公告通知</b>
            <el-tag type="info">共 {{ announcements.length }} 条</el-tag>
          </div>
        </template>
        <div v-if="announcements.length === 0" class="empty-tip">暂无公告</div>
        <div v-for="a in announcements" :key="a.announcement_id" class="ann-item">
          <div class="ann-head">
            <div class="ann-title">{{ a.pinned ? "📌 " : "" }}{{ a.title }}</div>
            <div class="ann-time">{{ fmtTime(a.created_at) }}</div>
          </div>
          <div class="md-body" v-html="renderMarkdown(a.content)" />
        </div>
      </el-card>
    </el-card>
  </div>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ApiClient, type Announcement } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { renderMarkdown } from "../../lib/markdown";

const loading = ref(false);
const error = ref("");
const guidelineContent = ref("正在加载用户准则...");
const announcements = ref<Announcement[]>([]);

function noticeSeenKey(): string {
  const u = String(authState.username || "").trim() || "anonymous";
  return `gpuops_seen_announcement_ts_${u}`;
}

function fmtTime(v: string): string {
  const d = new Date(v || "");
  if (Number.isNaN(d.getTime())) return v || "-";
  return d.toLocaleString();
}

function markAnnouncementSeen(rows: Announcement[]) {
  if (!rows.length) return;
  const latest = rows
    .map((x) => String(x.created_at || "").trim())
    .filter(Boolean)
    .sort()
    .at(-1);
  if (latest) {
    try {
      localStorage.setItem(noticeSeenKey(), latest);
      window.dispatchEvent(new CustomEvent("gpuops-announcement-seen", { detail: { latest } }));
    } catch {
      // ignore
    }
  }
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl);
    const [g, a] = await Promise.all([client.guideline(), client.announcements(100)]);
    guidelineContent.value = String(g.content || "").trim() || "暂无用户准则，请联系管理员。";
    announcements.value = a.announcements ?? [];
    markAnnouncementSeen(announcements.value);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

reload();
</script>

<style scoped>
.notice-card {
  min-height: 520px;
}
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.mb {
  margin-bottom: 12px;
}
.empty-tip {
  color: #64748b;
  font-size: 14px;
}
.ann-item {
  padding: 10px 0;
  border-bottom: 1px solid #e2e8f0;
}
.ann-item:last-child {
  border-bottom: none;
}
.ann-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 10px;
}
.ann-title {
  font-weight: 700;
  color: #0f172a;
}
.ann-time {
  font-size: 12px;
  color: #64748b;
}
.md-body :deep(p) { margin: 6px 0; color:#334155; }
.md-body :deep(h1), .md-body :deep(h2), .md-body :deep(h3), .md-body :deep(h4) { margin: 8px 0; color:#0f172a; }
.md-body :deep(ul), .md-body :deep(ol) { padding-left: 18px; margin: 6px 0; color:#334155; }
.md-body :deep(code) { background: #f1f5f9; padding: 1px 4px; border-radius: 4px; }
</style>
