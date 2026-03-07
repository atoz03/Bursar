<template>
  <el-card>
    <template #header>
      <div class="head">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><Document /></el-icon></span>
          <div>
          <div class="title">管理员记事本</div>
          <div class="sub">按日期记录、查看和修改管理事项</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="loading" @click="loadNotes">刷新</el-button>
          <el-button type="primary" @click="openCreateDialog">新增记事</el-button>
        </div>
      </div>
    </template>

    <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
    <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />

    <el-form inline class="mb">
      <el-form-item label="开始日期">
        <el-date-picker v-model="fromDate" type="date" value-format="YYYY-MM-DD" placeholder="开始日期" clearable />
      </el-form-item>
      <el-form-item label="结束日期">
        <el-date-picker v-model="toDate" type="date" value-format="YYYY-MM-DD" placeholder="结束日期" clearable />
      </el-form-item>
      <el-form-item>
        <el-button type="primary" @click="loadNotes">按日期查询</el-button>
        <el-button @click="resetFilter">重置</el-button>
      </el-form-item>
    </el-form>

    <el-table :data="rows" stripe height="560" empty-text="暂无记事">
      <el-table-column prop="note_date" label="日期" width="130" />
      <el-table-column prop="title" label="标题" min-width="180" />
      <el-table-column prop="content" label="内容" min-width="360" show-overflow-tooltip />
      <el-table-column prop="updated_by" label="更新人" width="130" />
      <el-table-column prop="updated_at" label="更新时间" min-width="180" :formatter="tableTimeFormatter" />
      <el-table-column label="操作" width="180" fixed="right">
        <template #default="{ row }">
          <el-space>
            <el-button size="small" @click="openEditDialog(row)">编辑</el-button>
            <el-button size="small" type="danger" @click="remove(row)">删除</el-button>
          </el-space>
        </template>
      </el-table-column>
    </el-table>

    <el-dialog v-model="editVisible" :title="editMode === 'create' ? '新增记事' : '编辑记事'" width="680px" destroy-on-close>
      <el-form label-width="84px">
        <el-form-item label="日期">
          <el-date-picker v-model="form.note_date" type="date" value-format="YYYY-MM-DD" placeholder="选择日期" style="width: 100%" />
        </el-form-item>
        <el-form-item label="标题">
          <el-input v-model="form.title" maxlength="200" show-word-limit placeholder="可选" />
        </el-form-item>
        <el-form-item label="内容">
          <el-input v-model="form.content" type="textarea" :rows="8" placeholder="请输入记事内容" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">
          {{ editMode === "create" ? "保存" : "保存修改" }}
        </el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { onMounted, reactive, ref } from "vue";
import { ElMessageBox } from "element-plus";
import { ApiClient, type AdminNote } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { Document } from "@element-plus/icons-vue";
import { formatServerDateTime } from "../../lib/time";

const loading = ref(false);
const saving = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<AdminNote[]>([]);
const fromDate = ref("");
const toDate = ref("");
const editVisible = ref(false);
const editMode = ref<"create" | "edit">("create");
const editingId = ref<number>(0);
const form = reactive({
  note_date: "",
  title: "",
  content: "",
});

function tableTimeFormatter(_: unknown, __: unknown, cellValue: unknown): string {
  return formatServerDateTime(String(cellValue ?? ""));
}

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function todayText(): string {
  const d = new Date();
  const y = d.getFullYear();
  const m = String(d.getMonth() + 1).padStart(2, "0");
  const day = String(d.getDate()).padStart(2, "0");
  return `${y}-${m}-${day}`;
}

function resetForm() {
  form.note_date = todayText();
  form.title = "";
  form.content = "";
  editingId.value = 0;
}

async function loadNotes() {
  loading.value = true;
  error.value = "";
  try {
    const r = await client().adminNotes({
      from: fromDate.value || "",
      to: toDate.value || "",
      limit: 2000,
    });
    rows.value = r.notes ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

function resetFilter() {
  fromDate.value = "";
  toDate.value = "";
  loadNotes();
}

function openCreateDialog() {
  editMode.value = "create";
  resetForm();
  editVisible.value = true;
}

function openEditDialog(row: AdminNote) {
  editMode.value = "edit";
  editingId.value = Number(row.note_id || 0);
  form.note_date = String(row.note_date || "").slice(0, 10);
  form.title = String(row.title || "");
  form.content = String(row.content || "");
  editVisible.value = true;
}

async function save() {
  error.value = "";
  success.value = "";
  const noteDate = String(form.note_date || "").trim();
  if (!noteDate) {
    error.value = "日期不能为空";
    return;
  }
  saving.value = true;
  try {
    if (editMode.value === "edit" && editingId.value > 0) {
      await client().adminUpdateNote(editingId.value, {
        note_date: noteDate,
        title: String(form.title || "").trim(),
        content: String(form.content || "").trim(),
      });
      success.value = "修改成功";
    } else {
      await client().adminCreateNote({
        note_date: noteDate,
        title: String(form.title || "").trim(),
        content: String(form.content || "").trim(),
      });
      success.value = "保存成功";
    }
    editVisible.value = false;
    await loadNotes();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    saving.value = false;
  }
}

async function remove(row: AdminNote) {
  error.value = "";
  success.value = "";
  try {
    await ElMessageBox.confirm(`确认删除 ${row.note_date} 的记事吗？`, "删除确认", {
      type: "warning",
      confirmButtonText: "确认删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  try {
    await client().adminDeleteNote(Number(row.note_id || 0));
    success.value = "删除成功";
    await loadNotes();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

onMounted(() => {
  resetForm();
  loadNotes();
});
</script>

<style scoped>
.head { display: flex; justify-content: space-between; align-items: center; gap: 12px; }
.row { display: flex; align-items: center; gap: 8px; }
.section-title-wrap { display: inline-flex; align-items: center; gap: 10px; }
.section-icon {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #dbeafe;
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
}
.title { font-weight: 700; }
.sub { margin-top: 4px; color: #64748b; font-size: 12px; }
.mb { margin-bottom: 12px; }
</style>
