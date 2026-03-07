<template>
  <div class="content-stack">
    <el-card class="section-card">
      <template #header>
        <div class="section-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-points"><el-icon><Coin /></el-icon></span>
            <div class="section-title-content">
              <div class="title">积分管理</div>
              <div class="sub">支持单用户调整、按筛选结果批量调整、全体加减、月初重置、特殊月度规则和完整操作记录</div>
            </div>
          </div>
          <el-button :loading="loading" type="primary" @click="reloadAll">刷新</el-button>
        </div>
      </template>

      <el-alert
        title="规则说明：当用户积分 ≤ -10 时，将强制中断其在所有关联节点上的进程。"
        type="warning"
        :closable="false"
        show-icon
        style="margin-bottom: 12px"
      />
      <el-alert v-if="error" :title="error" type="error" show-icon style="margin-bottom: 12px" />
      <el-alert v-if="success" :title="success" type="success" show-icon style="margin-bottom: 12px" />
      <el-alert
        title="支持按个人信息筛选：先选择匹配字段（或全字段），再输入关键词。筛选出的用户可直接批量加减积分。"
        type="info"
        :closable="false"
        show-icon
        style="margin-bottom: 12px"
      />
      <el-alert
        title="筛选排序说明：排序仅作用于当前匹配结果；输入筛选条件后，管理员账号不参与条件筛选。"
        type="info"
        :closable="false"
        show-icon
        style="margin-bottom: 12px"
      />

      <el-form inline>
        <el-form-item label="匹配字段">
          <el-select v-model="keywordField" style="width: 160px" @change="loadUsers">
            <el-option label="全字段" value="all" />
            <el-option label="用户名" value="username" />
            <el-option label="姓名" value="real_name" />
            <el-option label="学号" value="student_id" />
            <el-option label="导师" value="advisor" />
            <el-option label="邮箱" value="email" />
            <el-option label="手机号" value="phone" />
          </el-select>
        </el-form-item>
        <el-form-item label="用户查询">
          <el-autocomplete
            v-model="keyword"
            :fetch-suggestions="fetchPointsQuerySuggestions"
            :placeholder="keywordPlaceholder"
            clearable
            @select="onKeywordSelect"
            @change="onKeywordInputChange"
          />
        </el-form-item>
        <el-form-item>
          <el-button @click="loadUsers">查询</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="displayUsers" stripe height="420" @sort-change="onPointsUserSortChange">
        <el-table-column label="调整" width="94" fixed="left">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="openAdjust(row)">调整</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="total_balance" label="总积分" width="120" sortable="custom">
          <template #default="{ row }">{{ fmt2(row.total_balance ?? ((row.general_balance ?? row.balance) + (row.carryover_balance ?? 0) + (row.exclusive_balance ?? 0))) }}</template>
        </el-table-column>
        <el-table-column prop="general_balance" label="通用积分" width="120" sortable="custom">
          <template #default="{ row }">{{ fmt2(row.general_balance ?? row.balance) }}</template>
        </el-table-column>
        <el-table-column prop="carryover_balance" label="结转积分" width="120" sortable="custom">
          <template #default="{ row }">{{ fmt2(row.carryover_balance ?? 0) }}</template>
        </el-table-column>
        <el-table-column prop="exclusive_balance" label="专属积分" width="120" sortable="custom">
          <template #default="{ row }">{{ fmt2(row.exclusive_balance ?? 0) }}</template>
        </el-table-column>
        <el-table-column prop="username" label="用户名" width="180" show-overflow-tooltip sortable="custom">
          <template #default="{ row }">
            <el-button link type="primary" @click="openUserDetail(row.username)">{{ row.username }}</el-button>
          </template>
        </el-table-column>
        <el-table-column prop="role" label="用户级别" width="120" sortable="custom">
          <template #default="{ row }">
            <el-tag :type="roleTagType(row.role)">{{ roleText(row.role) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="120" sortable="custom" />
        <el-table-column prop="real_name" label="姓名" width="120" show-overflow-tooltip sortable="custom" />
        <el-table-column prop="student_id" label="学号" width="180" show-overflow-tooltip sortable="custom" />
        <el-table-column prop="advisor" label="导师" width="160" show-overflow-tooltip sortable="custom" />
        <el-table-column prop="email" label="邮箱" min-width="220" show-overflow-tooltip sortable="custom" />
        <el-table-column prop="phone" label="手机号" width="140" show-overflow-tooltip sortable="custom" />
      </el-table>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="section-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-filter-batch"><el-icon><UserFilled /></el-icon></span>
            <div class="section-title-content">
              <div class="title">按筛选结果批量加减</div>
              <div class="sub">对“当前查询结果”批量加减积分，可按个人信息先筛选，再统一执行</div>
            </div>
          </div>
          <el-tag type="info">当前命中 {{ filteredBatchTargetUsernames.length }} 人</el-tag>
        </div>
      </template>
      <el-form inline>
        <el-form-item>
          <el-checkbox v-model="filteredBatchOnlyRegular">仅普通用户</el-checkbox>
        </el-form-item>
        <el-form-item label="积分类别">
          <el-select v-model="filteredBatchScope" style="width: 160px">
            <el-option label="通用积分" value="general" />
            <el-option label="结转积分" value="carryover" />
            <el-option label="节点专属积分" value="node_exclusive" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="filteredBatchScope === 'node_exclusive'" label="节点编号">
          <el-input v-model="filteredBatchNodeId" placeholder="例如 60020" style="width: 160px" />
        </el-form-item>
        <el-form-item label="调整积分">
          <el-input-number v-model="filteredBatchAmount" :min="-1000000" :max="1000000" :step="10" />
        </el-form-item>
        <el-form-item label="设为固定值">
          <el-input-number v-model="filteredBatchSetValue" :min="-1000000" :max="1000000" :step="10" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="filteredBatchReason" placeholder="可选" />
        </el-form-item>
        <el-form-item>
          <el-button
            :loading="filteredBatchLoading"
            :disabled="filteredBatchTargetUsernames.length === 0"
            type="primary"
            @click="runFilteredBatchAdjust"
          >
            对当前筛选用户批量调整
          </el-button>
          <el-button
            :loading="filteredBatchSetLoading"
            :disabled="filteredBatchTargetUsernames.length === 0"
            type="warning"
            @click="runFilteredBatchSet"
          >
            对当前筛选用户设定固定值
          </el-button>
        </el-form-item>
      </el-form>
      <div class="sub">
        说明：按上方“用户查询”的筛选结果执行。可先按姓名/导师/学号等筛选，再批量加分或扣分。
      </div>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="section-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-batch"><el-icon><DataBoard /></el-icon></span>
            <div class="section-title-content">
              <div class="title">全体加减积分</div>
              <div class="sub">对所有普通用户统一加分或扣分（正数为加分，负数为扣分）</div>
            </div>
          </div>
        </div>
      </template>
      <el-form inline>
        <el-form-item label="积分类别">
          <el-select v-model="batchScope" style="width: 160px">
            <el-option label="通用积分" value="general" />
            <el-option label="结转积分" value="carryover" />
            <el-option label="节点专属积分" value="node_exclusive" />
          </el-select>
        </el-form-item>
        <el-form-item v-if="batchScope === 'node_exclusive'" label="节点编号">
          <el-input v-model="batchNodeId" placeholder="例如 60020" style="width: 160px" />
        </el-form-item>
        <el-form-item label="调整积分">
          <el-input-number v-model="batchAmount" :min="-1000000" :max="1000000" :step="10" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="batchReason" placeholder="可选" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="batchLoading" type="primary" @click="runBatchGrant">执行全体调整</el-button>
        </el-form-item>
      </el-form>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="section-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-record"><el-icon><Document /></el-icon></span>
            <div class="section-title-content">
              <div class="title">积分操作记录</div>
              <div class="sub">记录所有积分加减/重置动作（日期、操作、对象、变动值）</div>
            </div>
          </div>
          <el-tag type="info">共 {{ records.length }} 条</el-tag>
        </div>
      </template>
      <el-form inline>
        <el-form-item label="按平台账号筛选">
          <el-autocomplete
            v-model="recordKeyword"
            :fetch-suggestions="fetchUserSuggestions"
            placeholder="输入平台账号"
            clearable
            @select="onRecordKeywordSelect"
            @change="onRecordKeywordInputChange"
          />
        </el-form-item>
        <el-form-item>
          <el-button @click="loadRecords">查询记录</el-button>
        </el-form-item>
      </el-form>
      <el-table :data="records" stripe height="320">
        <el-table-column label="时间" min-width="180">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" min-width="160">
          <template #default="{ row }">{{ opLabel(row.method) }}</template>
        </el-table-column>
        <el-table-column label="对象账号" min-width="160">
          <template #default="{ row }">{{ targetAccountLabel(row) }}</template>
        </el-table-column>
        <el-table-column label="积分类型" width="130">
          <template #default="{ row }">{{ pointsScopeLabel(row.points_scope) }}</template>
        </el-table-column>
        <el-table-column label="节点编号" width="120">
          <template #default="{ row }">{{ row.node_id || "-" }}</template>
        </el-table-column>
        <el-table-column label="变动积分" width="140">
          <template #default="{ row }">
            <span :class="Number(row.amount || 0) >= 0 ? 'delta-plus' : 'delta-minus'">{{ fmtSigned(row.amount) }}</span>
          </template>
        </el-table-column>
        <el-table-column prop="method" label="操作编码" min-width="180" />
      </el-table>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="section-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-monthly"><el-icon><Clock /></el-icon></span>
            <div class="section-title-content">
              <div class="title">月初重置</div>
              <div class="sub">规则：学号含 B 按博士发放、含 S 按硕士发放、其余按“其他”发放；特殊用户规则优先；上月未用完通用积分按上限结转（上限为累计上限）；欠费超过“每月最大欠费上限”会强制清理全部进程</div>
            </div>
          </div>
          <div class="row">
            <el-tag type="info">当前月份：{{ monthlyStatus.month_key || "-" }}</el-tag>
            <el-tag :type="monthlyStatus.has_run ? 'success' : 'warning'">
              {{ monthlyStatus.has_run ? "本月已执行" : "本月未执行" }}
            </el-tag>
          </div>
        </div>
      </template>
      <el-form inline>
        <el-form-item label="博士(B)积分">
          <el-input-number v-model="doctorPoints" :min="0" :step="10" />
        </el-form-item>
        <el-form-item label="硕士(S)积分">
          <el-input-number v-model="masterPoints" :min="0" :step="10" />
        </el-form-item>
        <el-form-item label="其他积分">
          <el-input-number v-model="otherPoints" :min="0" :step="10" />
        </el-form-item>
        <el-form-item label="结转上限">
          <el-input-number v-model="carryoverLimit" :min="0" :step="10" />
        </el-form-item>
        <el-form-item label="每月最大欠费上限">
          <el-input-number v-model="maxOverdraftLimit" :min="0" :step="10" />
        </el-form-item>
        <el-form-item>
          <el-button :loading="configSaving" type="primary" @click="saveMonthlyConfig">保存发放规则</el-button>
        </el-form-item>
      </el-form>
      <el-form inline>
        <el-form-item>
          <el-checkbox v-model="forceReset">强制重置（覆盖本月已执行记录）</el-checkbox>
        </el-form-item>
        <el-form-item>
          <el-button :loading="monthlyResetLoading" type="primary" @click="runMonthlyReset">执行月初重置</el-button>
        </el-form-item>
      </el-form>
      <div class="sub">
        上次发放时间：{{ fmtDateTime(monthlyStatus.last_run?.run_at || "") }}
      </div>
      <div v-if="monthlyStatus.run?.run_at" class="sub">
        最近执行：{{ fmtDateTime(monthlyStatus.run?.run_at || "") }}，操作者：{{ monthlyStatus.run?.run_by || "-" }}，
        影响用户：{{ monthlyStatus.run?.changed_users || 0 }}/{{ monthlyStatus.run?.total_users || 0 }}
      </div>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="section-head">
          <div class="section-title-wrap">
            <span class="section-icon tone-rule"><el-icon><UserFilled /></el-icon></span>
            <div class="section-title-content">
              <div class="title">特殊用户月度积分</div>
              <div class="sub">为指定用户设置固定月度积分（优先于学号规则）</div>
            </div>
          </div>
        </div>
      </template>
      <el-form inline>
        <el-form-item label="用户名">
          <el-autocomplete
            v-model="ruleUsername"
            :fetch-suggestions="fetchUserSuggestions"
            placeholder="用户名"
            clearable
            @select="onRuleUsernameSelect"
            @change="onRuleUsernameChange"
          />
        </el-form-item>
        <el-form-item label="每月积分">
          <el-input-number v-model="rulePoints" :min="0" :step="10" />
        </el-form-item>
        <el-form-item>
          <el-checkbox v-model="ruleEnabled">启用</el-checkbox>
        </el-form-item>
        <el-form-item>
          <el-button :loading="ruleSaving" type="primary" @click="saveRule">新增/更新</el-button>
        </el-form-item>
      </el-form>

      <el-table :data="rules" stripe height="300">
        <el-table-column prop="username" label="用户名" width="180" />
        <el-table-column label="每月积分" width="120">
          <template #default="{ row }">{{ fmt2(row.monthly_points) }}</template>
        </el-table-column>
        <el-table-column label="启用" width="100">
          <template #default="{ row }">{{ row.enabled ? "是" : "否" }}</template>
        </el-table-column>
        <el-table-column prop="updated_by" label="更新人" width="140" />
        <el-table-column label="更新时间" min-width="180">
          <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="danger" plain @click="removeRule(row.username)">删除</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="adjustVisible" title="积分调整" width="540px">
      <el-form label-width="100px">
        <el-form-item label="用户名">
          <el-input v-model="adjustUsername" disabled />
        </el-form-item>
        <el-form-item label="当前积分">
          <div class="adjust-balance-wrap">
            <el-tag type="success" effect="plain">通用 {{ fmt2(adjustCurrentGeneralBalance) }}</el-tag>
            <el-tag type="info" effect="plain">结转 {{ fmt2(adjustCurrentCarryoverBalance) }}</el-tag>
            <el-tag type="warning" effect="plain">专属 {{ fmt2(adjustCurrentExclusiveBalance) }}</el-tag>
            <el-tag type="primary" effect="plain">总计 {{ fmt2(adjustCurrentTotalBalance) }}</el-tag>
          </div>
          <div v-if="adjustScope === 'node_exclusive'" class="sub">
            当前节点专属积分（节点 {{ adjustNodeId.trim() || "未填写" }}）：
            {{ adjustNodeExclusiveCurrent == null ? "-" : fmt2(adjustNodeExclusiveCurrent) }}
          </div>
          <div v-if="adjustBalanceLoading" class="sub">正在同步最新积分...</div>
          <div v-if="adjustBalanceError" class="sub adjust-balance-error">{{ adjustBalanceError }}</div>
        </el-form-item>
        <el-form-item label="调整值">
          <el-input-number v-model="adjustDelta" :step="10" :min="-1000000" :max="1000000" />
          <div class="sub">正数表示增加，负数表示减少</div>
        </el-form-item>
        <el-form-item label="积分类型">
          <el-radio-group v-model="adjustScope">
            <el-radio-button label="general">通用积分</el-radio-button>
            <el-radio-button label="carryover">结转积分</el-radio-button>
            <el-radio-button label="node_exclusive">节点专属积分</el-radio-button>
          </el-radio-group>
        </el-form-item>
        <el-form-item v-if="adjustScope === 'node_exclusive'" label="节点编号">
          <el-input v-model="adjustNodeId" placeholder="例如 60020" />
        </el-form-item>
        <el-form-item label="备注">
          <el-input v-model="adjustReason" placeholder="可选" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="adjustVisible = false">取消</el-button>
        <el-button :loading="adjustLoading" type="primary" @click="submitAdjust">确认</el-button>
      </template>
    </el-dialog>

    <PlatformUserDetailDialog v-model="detailVisible" :username="detailUsername" />
  </div>
</template>

<script setup lang="ts">
import { computed, onBeforeUnmount, onMounted, reactive, ref, watch } from "vue";
import { ElMessageBox } from "element-plus";
import { ApiClient, type PointsUser, type PointsOperationRecord, type SpecialMonthlyPointsRule, type MonthlyPointsResetRun } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime } from "../../lib/time";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Coin, DataBoard, Document, UserFilled } from "@element-plus/icons-vue";

type PointsKeywordField = "all" | "username" | "real_name" | "student_id" | "advisor" | "email" | "phone";
type PointsScope = "general" | "carryover" | "node_exclusive";
type SortOrder = "ascending" | "descending" | null;
type PointsUserSortKey =
  | "total_balance"
  | "general_balance"
  | "carryover_balance"
  | "exclusive_balance"
  | "username"
  | "role"
  | "status"
  | "real_name"
  | "student_id"
  | "advisor"
  | "email"
  | "phone";

const loading = ref(false);
const batchLoading = ref(false);
const monthlyResetLoading = ref(false);
const configSaving = ref(false);
const ruleSaving = ref(false);
const adjustLoading = ref(false);
const error = ref("");
const success = ref("");
const keyword = ref("");
const keywordField = ref<PointsKeywordField>("all");
const users = ref<PointsUser[]>([]);
const allUsers = ref<PointsUser[]>([]);
const records = ref<PointsOperationRecord[]>([]);
const rules = ref<SpecialMonthlyPointsRule[]>([]);
const monthlyStatus = reactive<{ month_key: string; has_run: boolean; run: MonthlyPointsResetRun | null; last_run: MonthlyPointsResetRun | null }>({
  month_key: "",
  has_run: false,
  run: null,
  last_run: null,
});

const batchAmount = ref(50);
const batchReason = ref("");
const batchScope = ref<PointsScope>("general");
const batchNodeId = ref("");
const filteredBatchAmount = ref(30);
const filteredBatchSetValue = ref(0);
const filteredBatchReason = ref("");
const filteredBatchScope = ref<PointsScope>("general");
const filteredBatchNodeId = ref("");
const filteredBatchLoading = ref(false);
const filteredBatchSetLoading = ref(false);
const filteredBatchOnlyRegular = ref(true);
const recordKeyword = ref("");
const forceReset = ref(false);
const doctorPoints = ref(200);
const masterPoints = ref(100);
const otherPoints = ref(50);
const carryoverLimit = ref(500);
const maxOverdraftLimit = ref(500);

const ruleUsername = ref("");
const rulePoints = ref(100);
const ruleEnabled = ref(true);

const adjustVisible = ref(false);
const adjustUsername = ref("");
const adjustDelta = ref(100);
const adjustReason = ref("");
const adjustScope = ref<"general" | "carryover" | "node_exclusive">("general");
const adjustNodeId = ref("");
const adjustBalanceLoading = ref(false);
const adjustBalanceError = ref("");
const adjustCurrentGeneralBalance = ref(0);
const adjustCurrentCarryoverBalance = ref(0);
const adjustCurrentExclusiveBalance = ref(0);
const adjustCurrentTotalBalance = ref(0);
const adjustNodeBalances = ref<Array<{ node_id: string; balance: number }>>([]);
const detailVisible = ref(false);
const detailUsername = ref("");
let usersQueryTimer: ReturnType<typeof setTimeout> | null = null;
let recordsQueryTimer: ReturnType<typeof setTimeout> | null = null;
const USERS_QUERY_DEBOUNCE_MS = 350;
const RECORDS_QUERY_DEBOUNCE_MS = 350;
const USERS_QUERY_LIMIT_EMPTY = 200;
const USERS_QUERY_LIMIT_FILTERED = 1000;
const ALL_USERS_LIMIT = 2000;
const RECORDS_QUERY_LIMIT = 300;
const adjustNodeExclusiveCurrent = computed<number | null>(() => {
  if (adjustScope.value !== "node_exclusive") return null;
  const nodeID = String(adjustNodeId.value || "").trim();
  if (!nodeID) return null;
  const row = adjustNodeBalances.value.find((x) => String(x.node_id || "").trim() === nodeID);
  return Number(row?.balance ?? 0);
});
const pointsUserSortKey = ref<PointsUserSortKey | "">("");
const pointsUserSortOrder = ref<SortOrder>(null);
const hasPointsFilter = computed(() => keyword.value.trim() !== "");
const keywordPlaceholder = computed(() => {
  if (keywordField.value === "username") return "输入用户名关键词";
  if (keywordField.value === "real_name") return "输入姓名关键词";
  if (keywordField.value === "student_id") return "输入学号关键词";
  if (keywordField.value === "advisor") return "输入导师关键词";
  if (keywordField.value === "email") return "输入邮箱关键词";
  if (keywordField.value === "phone") return "输入手机号关键词";
  return "用户名/姓名/学号/导师/邮箱/手机号";
});

const displayUsers = computed<PointsUser[]>(() => {
  const base = (users.value || []).filter((row) => !(hasPointsFilter.value && isAdminRole(row.role)));
  return sortPointsUsers(base, pointsUserSortKey.value, pointsUserSortOrder.value);
});

const filteredBatchTargetUsernames = computed<string[]>(() => {
  const base = displayUsers.value || [];
  const out: string[] = [];
  const seen = new Set<string>();
  for (const row of base) {
    const role = String(row.role || "").trim();
    if (filteredBatchOnlyRegular.value && role !== "user") continue;
    const username = String(row.username || "").trim();
    if (!username) continue;
    const key = username.toLowerCase();
    if (seen.has(key)) continue;
    seen.add(key);
    out.push(username);
  }
  return out;
});

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function normalizeUserKey(v: string): string {
  return String(v || "").trim().toLowerCase();
}

function isAdminRole(role?: string): boolean {
  return String(role || "").trim() === "admin";
}

function roleSortWeight(role?: string): number {
  const v = String(role || "").trim();
  if (v === "admin") return 0;
  if (v === "power_user") return 1;
  return 2;
}

function pointsUserTotalBalance(row: PointsUser): number {
  return Number(row.total_balance ?? ((row.general_balance ?? row.balance) + (row.carryover_balance ?? 0) + (row.exclusive_balance ?? 0)));
}

function compareNumberLike(a: number, b: number): number {
  return a === b ? 0 : (a < b ? -1 : 1);
}

function compareTextLike(a: string, b: string): number {
  return a.localeCompare(b, "zh-Hans-CN", { numeric: true, sensitivity: "base" });
}

function pointsUserSortValue(row: PointsUser, key: PointsUserSortKey): number | string {
  if (key === "total_balance") return pointsUserTotalBalance(row);
  if (key === "general_balance") return Number(row.general_balance ?? row.balance ?? 0);
  if (key === "carryover_balance") return Number(row.carryover_balance ?? 0);
  if (key === "exclusive_balance") return Number(row.exclusive_balance ?? 0);
  if (key === "role") return roleSortWeight(row.role);
  return String((row as any)?.[key] ?? "").trim();
}

function sortPointsUsers(rows: PointsUser[], key: PointsUserSortKey | "", order: SortOrder): PointsUser[] {
  const base = Array.isArray(rows) ? [...rows] : [];
  if (!key || !order) return base;
  const direction = order === "descending" ? -1 : 1;
  return base
    .map((row, index) => ({ row, index }))
    .sort((left, right) => {
      const a = pointsUserSortValue(left.row, key);
      const b = pointsUserSortValue(right.row, key);
      const delta = typeof a === "number" && typeof b === "number"
        ? compareNumberLike(a, b)
        : compareTextLike(String(a), String(b));
      if (delta !== 0) return delta * direction;
      return left.index - right.index;
    })
    .map((entry) => entry.row);
}

function dedupUsers(rows: PointsUser[]): PointsUser[] {
  const m = new Map<string, PointsUser>();
  for (const r of rows || []) {
    const key = normalizeUserKey(r.username);
    if (!key) continue;
    if (!m.has(key)) m.set(key, r);
  }
  return Array.from(m.values());
}

function fetchUserSuggestions(queryString: string, cb: (items: Array<{ value: string; username: string; student_id: string }>) => void) {
  const q = normalizeUserKey(queryString);
  const base = (allUsers.value.length ? allUsers.value : users.value) || [];
  const items = base
    .filter((u) => {
      if (!q) return true;
      const username = normalizeUserKey(u.username);
      const studentID = normalizeUserKey(u.student_id);
      const realName = normalizeUserKey(String(u.real_name || ""));
      const advisor = normalizeUserKey(String(u.advisor || ""));
      const email = normalizeUserKey(String(u.email || ""));
      const phone = normalizeUserKey(String(u.phone || ""));
      return (
        username.includes(q) ||
        studentID.includes(q) ||
        realName.includes(q) ||
        advisor.includes(q) ||
        email.includes(q) ||
        phone.includes(q)
      );
    })
    .slice(0, 30)
    .map((u) => ({
      value: String(u.username || "").trim(),
      username: String(u.username || "").trim(),
      student_id: String(u.student_id || "").trim(),
    }));
  cb(items);
}

function pointsKeywordFieldLabel(field: PointsKeywordField): string {
  if (field === "username") return "用户名";
  if (field === "real_name") return "姓名";
  if (field === "student_id") return "学号";
  if (field === "advisor") return "导师";
  if (field === "email") return "邮箱";
  if (field === "phone") return "手机号";
  return "全字段";
}

function pointsKeywordFieldValue(user: PointsUser, field: PointsKeywordField): string {
  if (field === "username") return String(user.username || "").trim();
  if (field === "real_name") return String(user.real_name || "").trim();
  if (field === "student_id") return String(user.student_id || "").trim();
  if (field === "advisor") return String(user.advisor || "").trim();
  if (field === "email") return String(user.email || "").trim();
  if (field === "phone") return String(user.phone || "").trim();
  return String(user.username || "").trim();
}

function matchesPointsKeyword(user: PointsUser, q: string, field: PointsKeywordField): boolean {
  const username = normalizeUserKey(user.username);
  const studentID = normalizeUserKey(user.student_id);
  const realName = normalizeUserKey(String(user.real_name || ""));
  const advisor = normalizeUserKey(String(user.advisor || ""));
  const email = normalizeUserKey(String(user.email || ""));
  const phone = normalizeUserKey(String(user.phone || ""));
  if (field === "username") return username.includes(q);
  if (field === "real_name") return realName.includes(q);
  if (field === "student_id") return studentID.includes(q);
  if (field === "advisor") return advisor.includes(q);
  if (field === "email") return email.includes(q);
  if (field === "phone") return phone.includes(q);
  return (
    username.includes(q) ||
    studentID.includes(q) ||
    realName.includes(q) ||
    advisor.includes(q) ||
    email.includes(q) ||
    phone.includes(q)
  );
}

function fetchPointsQuerySuggestions(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  const q = normalizeUserKey(queryString);
  const field = keywordField.value;
  const base = (allUsers.value.length ? allUsers.value : users.value) || [];
  const seen = new Set<string>();
  const items = base
    .filter((u) => {
      if (q && isAdminRole(u.role)) return false;
      if (!q) return true;
      return matchesPointsKeyword(u, q, field);
    })
    .map((u) => {
      const keyValue = pointsKeywordFieldValue(u, field);
      const value = keyValue || String(u.username || "").trim();
      const k = normalizeUserKey(value);
      if (!k || seen.has(k)) return null;
      seen.add(k);
      return { value };
    })
    .filter((x): x is { value: string } => !!x)
    .slice(0, 30);
  cb(items);
}

function extractSuggestionUsername(item: any): string {
  return String(item?.username || item?.value || "").trim();
}

function fmt2(v: number): string {
  return Number(v || 0).toFixed(2);
}

function fmtSigned(v: number): string {
  const n = Number(v || 0);
  return `${n >= 0 ? "+" : ""}${fmt2(n)}`;
}

function fmtTime(v: string): string {
  return formatServerDateTime(v);
}

function fmtDateTime(v: string): string {
  return formatServerDateTime(v);
}

function roleText(role: string): string {
  const v = String(role || "").trim();
  if (v === "admin") return "管理员";
  if (v === "power_user") return "高级用户";
  return "普通用户";
}

function roleTagType(role: string): "danger" | "warning" | "info" {
  const v = String(role || "").trim();
  if (v === "admin") return "danger";
  if (v === "power_user") return "warning";
  return "info";
}

function opLabel(method: string): string {
  const m = String(method || "").trim();
  if (m === "points_adjust_plus") return "单用户加分";
  if (m === "points_adjust_minus") return "单用户扣分";
  if (m === "points_adjust_carry_plus") return "单用户结转加分";
  if (m === "points_adjust_carry_minus") return "单用户结转扣分";
  if (m === "points_adjust_node_plus") return "单用户节点专属加分";
  if (m === "points_adjust_node_minus") return "单用户节点专属扣分";
  if (m === "points_batch_plus" || m === "points_batch_grant") return "全体加分";
  if (m === "points_batch_minus") return "全体扣分";
  if (m === "points_batch_carry_plus") return "全体结转加分";
  if (m === "points_batch_carry_minus") return "全体结转扣分";
  if (m === "points_batch_node_plus") return "全体节点专属加分";
  if (m === "points_batch_node_minus") return "全体节点专属扣分";
  if (m === "points_batch_filtered_plus") return "筛选批量加分";
  if (m === "points_batch_filtered_minus") return "筛选批量扣分";
  if (m === "points_batch_filtered_carry_plus") return "筛选批量结转加分";
  if (m === "points_batch_filtered_carry_minus") return "筛选批量结转扣分";
  if (m === "points_batch_filtered_node_plus") return "筛选批量节点专属加分";
  if (m === "points_batch_filtered_node_minus") return "筛选批量节点专属扣分";
  if (m === "points_batch_filtered_set") return "筛选批量设定通用积分";
  if (m === "points_batch_filtered_set_carry") return "筛选批量设定结转积分";
  if (m === "points_batch_filtered_set_node") return "筛选批量设定节点专属积分";
  if (m === "monthly_reset") return "月初重置";
  if (m === "monthly_carryover_reset") return "月初结转重置";
  return m || "-";
}

function pointsScopeLabel(scope: string): string {
  const s = String(scope || "").trim();
  if (s === "node_exclusive") return "节点专属";
  if (s === "carryover") return "结转";
  return "通用";
}

function pointsScopeText(scope: PointsScope, nodeID?: string): string {
  if (scope === "carryover") return "结转积分";
  if (scope === "node_exclusive") return `节点专属积分（节点 ${String(nodeID || "").trim() || "-"})`;
  return "通用积分";
}

function targetAccountLabel(row: PointsOperationRecord): string {
  const target = String(row.target_account || "").trim();
  if (target) return target;
  const method = String(row.method || "").trim();
  if (
    method === "points_batch_plus" ||
    method === "points_batch_minus" ||
    method === "points_batch_grant" ||
    method === "points_batch_carry_plus" ||
    method === "points_batch_carry_minus" ||
    method === "points_batch_node_plus" ||
    method === "points_batch_node_minus"
  ) {
    return "全部用户";
  }
  if (
    method === "points_batch_filtered_plus" ||
    method === "points_batch_filtered_minus" ||
    method === "points_batch_filtered_carry_plus" ||
    method === "points_batch_filtered_carry_minus" ||
    method === "points_batch_filtered_node_plus" ||
    method === "points_batch_filtered_node_minus" ||
    method === "points_batch_filtered_set" ||
    method === "points_batch_filtered_set_carry" ||
    method === "points_batch_filtered_set_node"
  ) {
    return "筛选用户组";
  }
  return String(row.username || "").trim() || "-";
}

function setAdjustCurrentBalances(
  generalBalance: number,
  carryoverBalance: number,
  exclusiveBalance: number,
  totalBalance?: number,
) {
  const g = Number(generalBalance || 0);
  const c = Number(carryoverBalance || 0);
  const e = Number(exclusiveBalance || 0);
  adjustCurrentGeneralBalance.value = g;
  adjustCurrentCarryoverBalance.value = c;
  adjustCurrentExclusiveBalance.value = e;
  adjustCurrentTotalBalance.value = Number(totalBalance ?? (g + c + e));
}

async function loadAdjustCurrentBalances(username: string) {
  const u = String(username || "").trim();
  if (!u) return;
  adjustBalanceLoading.value = true;
  adjustBalanceError.value = "";
  try {
    const r = await client().adminPlatformUserDetail(u);
    if (String(adjustUsername.value || "").trim() !== u) return;
    const user = r.user;
    const general = Number(user.general_balance ?? user.balance ?? 0);
    const carryover = Number(user.carryover_balance ?? 0);
    const exclusive = Number(user.exclusive_balance ?? 0);
    const total = Number(user.total_balance ?? (general + carryover + exclusive));
    setAdjustCurrentBalances(general, carryover, exclusive, total);
    adjustNodeBalances.value = (user.exclusive_balances ?? []).map((x) => ({
      node_id: String(x.node_id || "").trim(),
      balance: Number(x.balance || 0),
    }));
  } catch (e: any) {
    if (String(adjustUsername.value || "").trim() !== u) return;
    adjustBalanceError.value = `同步当前积分失败：${e?.message ?? String(e)}`;
  } finally {
    if (String(adjustUsername.value || "").trim() === u) {
      adjustBalanceLoading.value = false;
    }
  }
}

function clearUsersQueryTimer() {
  if (usersQueryTimer) {
    clearTimeout(usersQueryTimer);
    usersQueryTimer = null;
  }
}

function clearRecordsQueryTimer() {
  if (recordsQueryTimer) {
    clearTimeout(recordsQueryTimer);
    recordsQueryTimer = null;
  }
}

async function loadUsers() {
  const kw = String(keyword.value || "").trim();
  const limit = kw ? USERS_QUERY_LIMIT_FILTERED : USERS_QUERY_LIMIT_EMPTY;
  const r = await client().adminPointsUsers({ keyword: kw, keywordField: keywordField.value, limit });
  users.value = r.users ?? [];
}

async function loadAllUsers() {
  const r = await client().adminPointsUsers({ limit: ALL_USERS_LIMIT });
  allUsers.value = dedupUsers(r.users ?? []);
}

async function loadRecords() {
  const r = await client().adminPointsRecords({ username: String(recordKeyword.value || "").trim(), limit: RECORDS_QUERY_LIMIT });
  records.value = r.records ?? [];
}

async function loadRules() {
  const r = await client().adminPointsSpecialRules();
  rules.value = r.rules ?? [];
}

async function loadMonthlyStatus() {
  const r = await client().adminPointsMonthlyResetStatus();
  monthlyStatus.month_key = r.month_key;
  monthlyStatus.has_run = !!r.has_run;
  monthlyStatus.run = r.run || null;
  monthlyStatus.last_run = r.last_run || null;
}

async function loadMonthlyConfig() {
  const r = await client().adminPointsMonthlyConfig();
  doctorPoints.value = Number(r.config?.doctor_points ?? 200);
  masterPoints.value = Number(r.config?.master_points ?? 100);
  otherPoints.value = Number(r.config?.other_points ?? 50);
  carryoverLimit.value = Number(r.config?.carryover_limit ?? 500);
  maxOverdraftLimit.value = Number(r.config?.max_overdraft_limit ?? 500);
}

async function reloadAll() {
  loading.value = true;
  error.value = "";
  success.value = "";
  try {
    await Promise.all([loadUsers(), loadAllUsers(), loadRecords(), loadRules(), loadMonthlyStatus(), loadMonthlyConfig()]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function saveMonthlyConfig() {
  configSaving.value = true;
  error.value = "";
  success.value = "";
  try {
    await client().adminPointsSetMonthlyConfig({
      doctor_points: Number(doctorPoints.value || 0),
      master_points: Number(masterPoints.value || 0),
      other_points: Number(otherPoints.value || 0),
      carryover_limit: Number(carryoverLimit.value || 0),
      max_overdraft_limit: Number(maxOverdraftLimit.value || 0),
    });
    success.value = "月初发放/结转规则已保存，将在每月 1 号自动执行";
    await loadMonthlyConfig();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    configSaving.value = false;
  }
}

function openAdjust(row: PointsUser) {
  adjustUsername.value = row.username;
  adjustDelta.value = 100;
  adjustReason.value = "";
  adjustScope.value = "general";
  adjustNodeId.value = "";
  adjustBalanceError.value = "";
  adjustBalanceLoading.value = false;
  const general = Number(row.general_balance ?? row.balance ?? 0);
  const carryover = Number(row.carryover_balance ?? 0);
  const exclusive = Number(row.exclusive_balance ?? 0);
  const total = Number(row.total_balance ?? (general + carryover + exclusive));
  setAdjustCurrentBalances(general, carryover, exclusive, total);
  adjustNodeBalances.value = [];
  adjustVisible.value = true;
  void loadAdjustCurrentBalances(row.username);
}

function openUserDetail(username: string) {
  detailUsername.value = String(username || "").trim();
  if (!detailUsername.value) return;
  detailVisible.value = true;
}

async function submitAdjust() {
  if (!adjustUsername.value.trim()) return;
  if (!adjustDelta.value) {
    error.value = "调整值不能为 0";
    return;
  }
  if (adjustScope.value === "node_exclusive" && !adjustNodeId.value.trim()) {
    error.value = "节点专属积分模式下必须填写节点编号";
    return;
  }
  adjustLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminPointsAdjust({
      username: adjustUsername.value.trim(),
      delta: Number(adjustDelta.value),
      reason: adjustReason.value.trim(),
      scope: adjustScope.value,
      node_id: adjustNodeId.value.trim(),
    });
    adjustVisible.value = false;
    const label = r.scope === "node_exclusive"
      ? `节点专属积分（节点 ${r.node_id || "-"})`
      : (r.scope === "carryover" ? "结转积分" : "通用积分");
    if ((r.interrupt_targets || 0) > 0) {
      success.value = `${label}调整成功，已下发 ${r.interrupt_targets} 个节点账号的强制中断`;
    } else {
      success.value = `${label}调整成功`;
    }
    await reloadAll();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    adjustLoading.value = false;
  }
}

async function runFilteredBatchAdjust() {
  const amount = Number(filteredBatchAmount.value || 0);
  if (amount === 0) {
    error.value = "筛选批量调整值不能为 0";
    return;
  }
  const scope = filteredBatchScope.value;
  const nodeID = String(filteredBatchNodeId.value || "").trim();
  if (scope === "node_exclusive" && !nodeID) {
    error.value = "节点专属积分模式下必须填写节点编号";
    return;
  }
  const targets = filteredBatchTargetUsernames.value;
  if (targets.length === 0) {
    error.value = "当前筛选结果为空，无法批量调整";
    return;
  }
  const actionLabel = amount > 0 ? "筛选批量加分" : "筛选批量扣分";
  const scopeText = pointsScopeText(scope, nodeID);
  try {
    await ElMessageBox.confirm(
      `确认执行${actionLabel}吗？\n积分类别：${scopeText}\n筛选字段：${pointsKeywordFieldLabel(keywordField.value)}\n筛选关键词：${keyword.value.trim() || "（空）"}\n目标人数：${targets.length}\n调整值：${amount > 0 ? "+" : ""}${amount}\n备注：${filteredBatchReason.value.trim() || "（空）"}`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  filteredBatchLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminPointsBatchAdjustUsers({
      amount,
      reason: filteredBatchReason.value.trim(),
      usernames: targets,
      scope,
      node_id: scope === "node_exclusive" ? nodeID : undefined,
    });
    const successScopeText = pointsScopeText((r.scope as PointsScope) || scope, r.node_id || nodeID);
    success.value = `${actionLabel}（${successScopeText}）完成：命中 ${r.matched_users} 人，实际调整 ${r.adjusted_users} 人，总变动 ${fmt2(r.total_adjusted)} 积分`;
    if ((r.skipped_users || 0) > 0) {
      success.value += `；跳过 ${r.skipped_users} 人`;
    }
    if ((r.interrupted_users || 0) > 0) {
      success.value += `；强制中断 ${r.interrupted_users} 人（${r.interrupted_nodes} 节点账号）`;
    }
    await reloadAll();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    filteredBatchLoading.value = false;
  }
}

async function runFilteredBatchSet() {
  const value = Number(filteredBatchSetValue.value || 0);
  const scope = filteredBatchScope.value;
  const nodeID = String(filteredBatchNodeId.value || "").trim();
  if (scope === "node_exclusive" && !nodeID) {
    error.value = "节点专属积分模式下必须填写节点编号";
    return;
  }
  if ((scope === "carryover" || scope === "node_exclusive") && value < 0) {
    error.value = "结转积分和节点专属积分不能设为负数";
    return;
  }
  const targets = filteredBatchTargetUsernames.value;
  if (targets.length === 0) {
    error.value = "当前筛选结果为空，无法批量设定";
    return;
  }
  const scopeText = pointsScopeText(scope, nodeID);
  try {
    await ElMessageBox.confirm(
      `确认按固定值设定吗？\n积分类别：${scopeText}\n筛选字段：${pointsKeywordFieldLabel(keywordField.value)}\n筛选关键词：${keyword.value.trim() || "（空）"}\n目标人数：${targets.length}\n设定值：${fmt2(value)}\n备注：${filteredBatchReason.value.trim() || "（空）"}`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  filteredBatchSetLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminPointsBatchSetUsers({
      value,
      reason: filteredBatchReason.value.trim(),
      usernames: targets,
      scope,
      node_id: scope === "node_exclusive" ? nodeID : undefined,
    });
    const successScopeText = pointsScopeText((r.scope as PointsScope) || scope, r.node_id || nodeID);
    success.value = `筛选批量设定（${successScopeText}）完成：命中 ${r.matched_users} 人，变更 ${r.changed_users} 人，未变化 ${r.unchanged_users} 人，总变动 ${fmtSigned(r.total_delta)} 积分`;
    if ((r.skipped_users || 0) > 0) {
      success.value += `；跳过 ${r.skipped_users} 人`;
    }
    if ((r.interrupted_users || 0) > 0) {
      success.value += `；强制中断 ${r.interrupted_users} 人（${r.interrupted_nodes} 节点账号）`;
    }
    await reloadAll();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    filteredBatchSetLoading.value = false;
  }
}

async function runBatchGrant() {
  const amount = Number(batchAmount.value || 0);
  if (amount === 0) {
    error.value = "全体调整值不能为 0";
    return;
  }
  const scope = batchScope.value;
  const nodeID = String(batchNodeId.value || "").trim();
  if (scope === "node_exclusive" && !nodeID) {
    error.value = "节点专属积分模式下必须填写节点编号";
    return;
  }
  const actionLabel = amount > 0 ? "全体加分" : "全体扣分";
  const scopeText = pointsScopeText(scope, nodeID);
  try {
    await ElMessageBox.confirm(
      `确认执行${actionLabel}吗？\n积分类别：${scopeText}\n调整值：${amount > 0 ? "+" : ""}${amount}\n备注：${batchReason.value.trim() || "（空）"}`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认执行", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  batchLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminPointsBatchGrant({
      amount,
      reason: batchReason.value.trim(),
      scope,
      node_id: scope === "node_exclusive" ? nodeID : undefined,
    });
    const successScopeText = pointsScopeText((r.scope as PointsScope) || scope, r.node_id || nodeID);
    success.value = `${actionLabel}（${successScopeText}）完成：${r.adjusted_users} 人，总变动 ${fmt2(r.total_adjusted)} 积分`;
    if ((r.interrupted_users || 0) > 0) {
      success.value += `；强制中断 ${r.interrupted_users} 人（${r.interrupted_nodes} 节点账号）`;
    }
    await reloadAll();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    batchLoading.value = false;
  }
}

async function runMonthlyReset() {
  try {
    await ElMessageBox.confirm(
      forceReset.value
        ? "确认执行“强制月初重置”吗？将覆盖本月已执行记录并重新发放积分。"
        : "确认执行“月初重置”吗？将按当前规则发放积分。",
      "二次确认",
      {
        type: "warning",
        confirmButtonText: "确认执行",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  monthlyResetLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminPointsMonthlyReset(forceReset.value);
    success.value = `${r.message}：影响 ${r.changed_users}/${r.total_users} 人，强制中断 ${r.interrupted_users} 人（${r.interrupted_nodes} 节点账号）`;
    monthlyStatus.month_key = r.month_key;
    monthlyStatus.has_run = true;
    monthlyStatus.run = r.run || monthlyStatus.run;
    await reloadAll();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    monthlyResetLoading.value = false;
  }
}

async function saveRule() {
  const username = ruleUsername.value.trim();
  if (!username) {
    error.value = "用户名不能为空";
    return;
  }
  ruleSaving.value = true;
  error.value = "";
  success.value = "";
  try {
    await client().adminPointsUpsertSpecialRule({
      username,
      monthly_points: Number(rulePoints.value || 0),
      enabled: !!ruleEnabled.value,
    });
    success.value = "特殊用户规则已保存";
    ruleUsername.value = "";
    rulePoints.value = 100;
    ruleEnabled.value = true;
    await loadRules();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    ruleSaving.value = false;
  }
}

function autoFillRuleForUsername(rawUsername: string) {
  const username = String(rawUsername || "").trim();
  if (!username) return;
  const hit = rules.value.find((r) => String(r.username || "").trim() === username);
  if (hit) {
    rulePoints.value = Number(hit.monthly_points || 0);
    ruleEnabled.value = !!hit.enabled;
    return;
  }
  const user = allUsers.value.find((u) => String(u.username || "").trim() === username);
  if (!user) return;
  const sid = String(user.student_id || "").toUpperCase();
  if (sid.includes("B")) {
    rulePoints.value = Number(doctorPoints.value || 0);
  } else if (sid.includes("S")) {
    rulePoints.value = Number(masterPoints.value || 0);
  } else {
    rulePoints.value = Number(otherPoints.value || 0);
  }
  ruleEnabled.value = true;
}

function onKeywordSelect(item: any) {
  keyword.value = extractSuggestionUsername(item);
  clearUsersQueryTimer();
  void loadUsers();
}

function onPointsUserSortChange({ prop, order }: { prop: string; order: SortOrder }) {
  if (!prop || !order) {
    pointsUserSortKey.value = "";
    pointsUserSortOrder.value = null;
    return;
  }
  pointsUserSortKey.value = prop as PointsUserSortKey;
  pointsUserSortOrder.value = order;
}

function onRecordKeywordSelect(item: any) {
  recordKeyword.value = extractSuggestionUsername(item);
  clearRecordsQueryTimer();
  void loadRecords();
}

function onRuleUsernameSelect(item: any) {
  ruleUsername.value = extractSuggestionUsername(item);
  autoFillRuleForUsername(ruleUsername.value);
}

function onRuleUsernameChange() {
  autoFillRuleForUsername(ruleUsername.value);
}

function onKeywordInputChange() {
  const kw = String(keyword.value || "").trim();
  clearUsersQueryTimer();
  usersQueryTimer = setTimeout(() => {
    if (kw.length === 0 || kw.length >= 2) {
      void loadUsers();
    }
  }, USERS_QUERY_DEBOUNCE_MS);
}

function onRecordKeywordInputChange() {
  const kw = String(recordKeyword.value || "").trim();
  clearRecordsQueryTimer();
  recordsQueryTimer = setTimeout(() => {
    if (kw.length === 0 || kw.length >= 2) {
      void loadRecords();
    }
  }, RECORDS_QUERY_DEBOUNCE_MS);
}

async function removeRule(username: string) {
  try {
    await ElMessageBox.confirm(`确认删除特殊规则用户 ${username} 吗？`, "二次确认", {
      type: "warning",
      confirmButtonText: "确认删除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  error.value = "";
  success.value = "";
  try {
    await client().adminPointsDeleteSpecialRule(username);
    success.value = "规则删除成功";
    await loadRules();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

onMounted(async () => {
  await reloadAll();
});

watch(
  () => keyword.value,
  (v) => {
    if (!String(v || "").trim()) {
      clearUsersQueryTimer();
      void loadUsers();
    }
  },
);

watch(
  () => recordKeyword.value,
  (v) => {
    if (!String(v || "").trim()) {
      clearRecordsQueryTimer();
      void loadRecords();
    }
  },
);

onBeforeUnmount(() => {
  clearUsersQueryTimer();
  clearRecordsQueryTimer();
});
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
.section-card {
  border: 1px solid var(--border-color);
  box-shadow: 0 8px 24px rgba(15, 23, 42, 0.06);
}
.section-card :deep(.el-card__header) {
  padding: 12px 16px;
  background: #f7fbff;
  border-bottom: 1px solid var(--border-color);
}
.section-head {
  display: flex;
  align-items: center;
  justify-content: space-between;
  gap: 12px;
}
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.section-title-content {
  display: flex;
  flex-direction: column;
  justify-content: center;
  gap: 2px;
}
.section-icon {
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  flex-shrink: 0;
  box-shadow: inset 0 0 0 1px rgba(255, 255, 255, 0.35);
}
.section-icon :deep(svg) {
  width: 16px;
  height: 16px;
}
.tone-points {
  background: linear-gradient(135deg, #f59e0b, #f97316);
  color: #fff7ed;
}
.tone-batch {
  background: linear-gradient(135deg, #2563eb, #1d4ed8);
  color: #dbeafe;
}
.tone-filter-batch {
  background: linear-gradient(135deg, #0f766e, #115e59);
  color: #ccfbf1;
}
.tone-record {
  background: linear-gradient(135deg, #0f766e, #0d9488);
  color: #ccfbf1;
}
.tone-monthly {
  background: linear-gradient(135deg, #7c3aed, #6d28d9);
  color: #ede9fe;
}
.tone-rule {
  background: linear-gradient(135deg, #be123c, #e11d48);
  color: #ffe4e6;
}

.title {
  font-weight: 700;
}

.sub {
  color: var(--text-tertiary);
  font-size: 12px;
}
.adjust-balance-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.adjust-balance-error {
  color: var(--error-color);
}
.delta-plus {
  color: #15803d;
  font-weight: 700;
}
.delta-minus {
  color: #b91c1c;
  font-weight: 700;
}
@media (max-width: 900px) {
  .section-head {
    flex-wrap: wrap;
  }
}
</style>
