<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><Coin /></el-icon></span>
          <div>
          <div class="title">价格配置</div>
          <div class="sub">配置 CPU 与 GPU 模型的积分单价</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
          <el-button @click="openEdit()">新增/修改</el-button>
        </div>
      </div>
    </template>

    <div class="content-stack">
      <el-alert v-if="error" :title="error" type="error" show-icon />

      <el-table :data="rows" stripe height="520">
        <el-table-column prop="model" label="模型关键词" width="260" />
        <el-table-column prop="price" label="积分/分钟" width="160" />
        <el-table-column label="操作" width="200">
          <template #default="{ row }">
            <el-button size="small" @click="openEdit(row.model, row.price)">编辑</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="editVisible" title="设置积分单价" width="420px">
      <el-form label-width="110px">
        <el-form-item label="模型关键词">
          <el-input v-model="editModel" placeholder="例如 RTX 3090 / A100 / CPU_CORE" />
        </el-form-item>
        <el-form-item label="积分/分钟">
          <el-input-number v-model="editPrice" :min="0" :max="1000" :step="0.01" :precision="4" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button :loading="editLoading" type="primary" @click="save">保存</el-button>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { ref } from "vue";
import { ApiClient } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { Coin } from "@element-plus/icons-vue";

type PriceRow = { model: string; price: number };

const loading = ref(false);
const error = ref("");
const rows = ref<PriceRow[]>([]);

const editVisible = ref(false);
const editLoading = ref(false);
const editModel = ref("");
const editPrice = ref(0.1);

function toRow(p: any): PriceRow {
  return {
    model: (p?.model ?? p?.Model ?? "").toString(),
    price: Number(p?.price ?? p?.Price ?? 0),
  };
}

async function reload() {
  loading.value = true;
  error.value = "";
  rows.value = [];
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminPrices();
    rows.value = (r.prices ?? []).map(toRow).sort((a, b) => a.model.localeCompare(b.model));
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    loading.value = false;
  }
}

function openEdit(model = "", price = 0.1) {
  editModel.value = model;
  editPrice.value = price;
  editVisible.value = true;
}

async function save() {
  editLoading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminSetPrice(editModel.value.trim(), editPrice.value);
    editVisible.value = false;
    await reload();
  } catch (e: any) {
    error.value = e?.body ? `${e.message}\n${e.body}` : (e?.message ?? String(e));
  } finally {
    editLoading.value = false;
  }
}

reload();
</script>

<style scoped>
.row {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.content-stack {
  width: 100%;
  min-width: 0;
  display: flex;
  flex-direction: column;
  gap: 12px;
}
.title {
  font-weight: 700;
}
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.section-icon {
  width: 26px;
  height: 26px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #fef3c7;
  background: linear-gradient(135deg, #a16207, #d97706);
  flex-shrink: 0;
}
.sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}
</style>
