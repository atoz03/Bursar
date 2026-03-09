<template>
  <el-card>
    <template #header>
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon"><el-icon><UserFilled /></el-icon></span>
          <div>
          <div class="title">平台用户管理</div>
          <div class="sub">平台账号资料、状态控制、删除恢复与重复身份排查</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="loading" type="primary" @click="reload">刷新</el-button>
        </div>
      </div>
    </template>

    <div class="content-stack">
      <el-alert v-if="error" :title="error" type="error" show-icon />
      <el-alert v-if="success" :title="success" type="success" show-icon />
      <div class="section-title-wrap">
        <span class="section-icon tone-list"><el-icon><List /></el-icon></span>
        <div>
          <div class="title">平台账号列表</div>
          <div class="sub">支持字段筛选、排序、改分、拉黑/解黑、删除和重复身份排查。</div>
        </div>
      </div>
      <el-form inline>
        <el-form-item label="匹配字段">
          <el-select v-model="keywordField" style="width: 170px">
            <el-option label="全字段" value="all" />
            <el-option label="平台账号" value="username" />
            <el-option label="平台UID" value="platform_uid" />
            <el-option label="真实姓名" value="real_name" />
            <el-option label="学号" value="student_id" />
            <el-option label="导师" value="advisor" />
            <el-option label="邮箱" value="email" />
            <el-option label="手机号" value="phone" />
            <el-option label="角色" value="role" />
            <el-option label="状态" value="status" />
          </el-select>
        </el-form-item>
        <el-form-item label="平台账号查询">
          <el-input v-model="keyword" :placeholder="keywordPlaceholder" clearable />
        </el-form-item>
      </el-form>
      <el-alert
        title="筛选排序说明：未筛选时管理员始终置顶；输入筛选条件后，管理员不参与条件筛选。排序仅作用于当前匹配结果。"
        type="info"
        :closable="false"
        show-icon
        class="mb"
      />

      <el-table :data="filteredRows" stripe height="520" empty-text="暂无数据" @sort-change="onAdminUserSortChange">
        <el-table-column type="expand">
          <template #default="{ row }">
            <div class="expand-wrap">
              <el-descriptions :column="3" border>
                <el-descriptions-item label="平台UID">{{ row.platform_uid ?? "-" }}</el-descriptions-item>
                <el-descriptions-item label="真实姓名">{{ row.real_name || "-" }}</el-descriptions-item>
                <el-descriptions-item label="导师">{{ row.advisor || "-" }}</el-descriptions-item>
                <el-descriptions-item label="预计毕业">{{ fmtGrad(row.expected_graduation_year, row.expected_graduation_month) }}</el-descriptions-item>
                <el-descriptions-item label="手机号">{{ row.phone || "-" }}</el-descriptions-item>
                <el-descriptions-item label="累计记录">{{ row.usage_records }}</el-descriptions-item>
                <el-descriptions-item label="累计积分消耗">{{ fmt2(row.total_cost) }}</el-descriptions-item>
                <el-descriptions-item label="最后使用时间">{{ fmtTime(row.last_usage_at) }}</el-descriptions-item>
                <el-descriptions-item label="运营看板权限">{{ row.can_view_board ? "有" : "无" }}</el-descriptions-item>
                <el-descriptions-item label="节点状态权限">{{ row.can_view_nodes ? "有" : "无" }}</el-descriptions-item>
                <el-descriptions-item label="注册审核权限">{{ row.can_review_requests ? "有" : "无" }}</el-descriptions-item>
              </el-descriptions>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="username" label="平台账号" width="160" sortable="custom">
          <template #default="{ row }">
            <span class="username-cell">
              <button type="button" class="username-link" @click="openProfile(row.username)">{{ row.username }}</button>
              <span v-if="isBlack(row)" class="black-badge">黑</span>
              <span v-if="isWhite(row)" class="white-badge">白</span>
              <span v-if="isExempt(row)" class="exempt-badge">豁</span>
            </span>
          </template>
        </el-table-column>
        <el-table-column prop="platform_uid" label="平台UID" width="110" sortable="custom">
          <template #default="{ row }">{{ row.platform_uid ?? "-" }}</template>
        </el-table-column>
        <el-table-column prop="real_name" label="真实姓名" width="120" sortable="custom" />
        <el-table-column prop="role" label="账号类型" width="110" sortable="custom">
          <template #default="{ row }">
            {{ roleText(row.role) }}
          </template>
        </el-table-column>
        <el-table-column prop="two_factor_enabled" label="2FA" width="100" sortable="custom">
          <template #default="{ row }">
            <el-tag size="small" :type="row.two_factor_enabled ? 'success' : 'info'">
              {{ row.two_factor_enabled ? "已开启" : "未开启" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="student_id" label="学号" width="160" sortable="custom" />
        <el-table-column prop="email" label="邮箱" min-width="220" sortable="custom" />
        <el-table-column prop="expected_graduation_year" label="预计毕业" width="120" sortable="custom">
          <template #default="{ row }">{{ fmtGrad(row.expected_graduation_year, row.expected_graduation_month) }}</template>
        </el-table-column>
        <el-table-column prop="balance" label="积分余额" width="120" sortable="custom">
          <template #default="{ row }">{{ fmt2(row.balance) }}</template>
        </el-table-column>
        <el-table-column prop="status" label="状态" width="100" sortable="custom">
          <template #default="{ row }">{{ effectiveStatus(row) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="420" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button size="small" :disabled="row.role !== 'user'" @click="openRecharge(row)">改分</el-button>
              <el-button v-if="!isBlack(row)" size="small" type="danger" :disabled="row.role !== 'user'" @click="blockUser(row)">拉黑</el-button>
              <el-button v-else size="small" type="success" :disabled="row.role !== 'user'" @click="unblockUser(row)">解黑</el-button>
              <el-button size="small" type="warning" :disabled="row.role !== 'user'" @click="deleteUser(row)">删除</el-button>
              <el-button size="small" @click="queryDuplicates(row)">查重</el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>

      <el-divider />
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon tone-delete"><el-icon><Delete /></el-icon></span>
          <div>
          <div class="title">已删除平台账号（可恢复）</div>
          <div class="sub">恢复时会校验当前已注册与待审核注册申请，冲突会明确提示。</div>
          </div>
        </div>
      </div>
      <el-table :data="deletedRows" stripe height="280" empty-text="暂无数据">
        <el-table-column prop="deleted_id" label="删除ID" width="90" />
        <el-table-column prop="username" label="平台账号" width="150" />
        <el-table-column label="平台UID" width="110">
          <template #default="{ row }">{{ row.platform_uid ?? "-" }}</template>
        </el-table-column>
        <el-table-column prop="real_name" label="真实姓名" width="120" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column prop="delete_reason" label="删除原因" min-width="220" />
        <el-table-column prop="deleted_by" label="删除人" width="120" />
        <el-table-column label="删除时间" width="180">
          <template #default="{ row }">{{ fmtTime(row.deleted_at) }}</template>
        </el-table-column>
        <el-table-column label="UID释放" min-width="220">
          <template #default="{ row }">{{ fmtUIDRelease(row) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="150" fixed="right">
          <template #default="{ row }">
            <el-button size="small" type="primary" @click="restoreUser(row)">恢复</el-button>
          </template>
        </el-table-column>
      </el-table>

      <el-divider />
      <div class="row">
        <div class="section-title-wrap">
          <span class="section-icon tone-remind"><el-icon><Bell /></el-icon></span>
          <div>
          <div class="title">毕业到期提醒（两个月后可能清理数据）</div>
          <div class="sub">查询已达到预计毕业时间的用户，可单发或批量发送备份提醒邮件。</div>
          </div>
        </div>
        <div class="row">
          <el-button :loading="dueLoading" @click="loadGraduationDueUsers">查询毕业到期用户</el-button>
          <el-button :loading="sendLoading" type="primary" @click="sendSelectedDueUsers">发送已选中</el-button>
          <el-button :loading="sendLoading" type="warning" @click="sendAllDueUsers">发送全部到期用户</el-button>
          <el-button :loading="dueDeleteLoading" type="danger" @click="deleteSelectedDueUsers">
            <el-icon><Check /></el-icon>
            <span style="margin-left: 4px">勾选删除账号</span>
          </el-button>
        </div>
      </div>
      <el-table :data="dueRows" stripe height="320" @selection-change="onDueSelectionChange" empty-text="暂无数据">
        <el-table-column type="selection" width="48" />
        <el-table-column prop="username" label="用户名" width="140" />
        <el-table-column prop="student_id" label="学号" width="140" />
        <el-table-column prop="email" label="邮箱" min-width="220" />
        <el-table-column label="预计毕业" width="120">
          <template #default="{ row }">{{ fmtGrad(row.expected_graduation_year, row.expected_graduation_month) }}</template>
        </el-table-column>
        <el-table-column prop="overdue_months" label="已到期(月)" width="100" />
        <el-table-column label="发送状态" min-width="220">
          <template #default="{ row }">
            <span :style="{ color: row.send_success ? '#16a34a' : (row.send_error ? '#dc2626' : '#475569') }">
              {{ row.send_success ? "发送成功" : (row.send_error || "未发送") }}
            </span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120">
          <template #default="{ row }">
            <el-button size="small" type="primary" :loading="sendLoading" @click="sendSingleDueUser(row)">发送</el-button>
          </template>
        </el-table-column>
      </el-table>
    </div>

    <el-dialog v-model="rechargeVisible" title="积分调整" width="500px">
      <el-form label-width="90px">
        <el-form-item label="用户名">
          <el-input v-model="rechargeUser" disabled />
        </el-form-item>
        <el-form-item label="当前积分">
          <div class="recharge-balance-wrap">
            <el-tag type="success" effect="plain">通用 {{ fmt2(rechargeGeneralBalance) }}</el-tag>
            <el-tag type="info" effect="plain">结转 {{ fmt2(rechargeCarryoverBalance) }}</el-tag>
            <el-tag type="warning" effect="plain">专属 {{ fmt2(rechargeExclusiveBalance) }}</el-tag>
            <el-tag type="primary" effect="plain">总计 {{ fmt2(rechargeTotalBalance) }}</el-tag>
          </div>
          <div v-if="rechargeBalanceLoading" class="sub">正在同步最新积分...</div>
          <div v-if="rechargeBalanceError" class="sub recharge-balance-error">{{ rechargeBalanceError }}</div>
        </el-form-item>
        <el-form-item label="调整值">
          <el-input-number v-model="rechargeAmount" :min="-100000" :max="100000" :step="10" />
          <div class="sub">正数表示加分，负数表示扣分</div>
        </el-form-item>
        <el-form-item label="方式">
          <el-input v-model="rechargeMethod" placeholder="admin/wechat/alipay" />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="rechargeVisible = false">取消</el-button>
        <el-button :loading="rechargeLoading" type="primary" @click="doRecharge">确认</el-button>
      </template>
    </el-dialog>

    <el-dialog v-model="profileVisible" width="680px">
      <template #header>
        <div class="dialog-title-wrap">
          <span class="section-icon tone-profile"><el-icon><UserFilled /></el-icon></span>
          <span class="title">平台账号信息</span>
        </div>
      </template>
      <el-alert v-if="profileError" :title="profileError" type="error" show-icon />
      <el-skeleton v-else-if="profileLoading" :rows="6" animated />
      <el-descriptions v-else-if="profileData" :column="2" border>
        <el-descriptions-item label="平台账号">{{ profileData.username }}</el-descriptions-item>
        <el-descriptions-item label="平台UID">{{ profileData.platform_uid ?? "-" }}</el-descriptions-item>
        <el-descriptions-item label="真实姓名">{{ profileData.real_name || "-" }}</el-descriptions-item>
        <el-descriptions-item label="学号">{{ profileData.student_id || "-" }}</el-descriptions-item>
        <el-descriptions-item label="邮箱">{{ profileData.email || "-" }}</el-descriptions-item>
        <el-descriptions-item label="导师">{{ profileData.advisor || "-" }}</el-descriptions-item>
        <el-descriptions-item label="电话">{{ profileData.phone || "-" }}</el-descriptions-item>
        <el-descriptions-item label="预计毕业">{{ fmtGrad(profileData.expected_graduation_year, profileData.expected_graduation_month) }}</el-descriptions-item>
        <el-descriptions-item label="通用积分">{{ fmt2(profileData.general_balance ?? profileData.balance) }}</el-descriptions-item>
        <el-descriptions-item label="结转积分">{{ fmt2(profileData.carryover_balance ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="节点专属积分">{{ fmt2(profileData.exclusive_balance ?? 0) }}</el-descriptions-item>
        <el-descriptions-item label="总可用积分">{{ fmt2(profileData.total_balance ?? ((profileData.general_balance ?? profileData.balance) + (profileData.carryover_balance ?? 0) + (profileData.exclusive_balance ?? 0))) }}</el-descriptions-item>
        <el-descriptions-item label="状态">{{ profileData.status || "-" }}</el-descriptions-item>
        <el-descriptions-item label="角色">{{ profileData.role || "-" }}</el-descriptions-item>
        <el-descriptions-item label="2FA 状态">
          <el-tag :type="profileData.two_factor_enabled ? 'success' : 'info'">{{ profileData.two_factor_enabled ? "已开启" : "未开启" }}</el-tag>
        </el-descriptions-item>
      </el-descriptions>
      <template v-if="profileData">
        <div class="section-title-wrap mt10">
          <span class="section-icon tone-profile"><el-icon><UserFilled /></el-icon></span>
          <div class="title">账号安全</div>
        </div>
        <el-alert
          :title="profileData.two_factor_enabled ? '该账号当前已启用 2FA。登录时必须同时校验密码、登录验证码和动态码。' : '该账号当前未启用 2FA。如需开启，请由用户本人登录后自行配置。'"
          :type="profileData.two_factor_enabled ? 'success' : 'warning'"
          show-icon
          :closable="false"
          class="mb"
        />

        <div class="section-title-wrap mt10">
          <span class="section-icon tone-map"><el-icon><Connection /></el-icon></span>
          <div class="title">节点账号映射</div>
        </div>
        <el-table :data="profileData.node_accounts || []" stripe max-height="220" empty-text="暂无映射">
          <el-table-column prop="node_id" label="节点编号" width="140" />
          <el-table-column prop="local_username" label="节点账号" width="160" />
          <el-table-column label="状态" width="220">
            <template #default="{ row }">
              <div class="mapping-state-cell">
                <el-tag v-if="row.identity_aligned" type="success" effect="light">已就绪</el-tag>
                <el-tag v-else-if="row.identity_initializing" type="warning" effect="light">初始化中</el-tag>
                <el-tag v-else type="info" effect="light">待同步</el-tag>
                <div v-if="mappingStateTip(row)" class="mapping-state-tip">{{ mappingStateTip(row) }}</div>
              </div>
            </template>
          </el-table-column>
          <el-table-column label="更新时间" min-width="180">
            <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
          </el-table-column>
          <el-table-column label="操作" min-width="300">
            <template #default="{ row: acc }">
              <el-button
                v-if="!isNodeAccountBlack(acc?.node_id, acc?.local_username)"
                size="small"
                type="danger"
                @click="disableNodeAccountMapping(acc)"
              >禁用</el-button>
              <el-button
                v-else
                size="small"
                type="success"
                @click="enableNodeAccountMapping(acc)"
              >解除禁用</el-button>
              <el-button
                size="small"
                type="warning"
                plain
                @click="submitProfileUnbindRequest(acc)"
              >代提解绑</el-button>
              <el-button
                size="small"
                type="danger"
                plain
                @click="forceUnbindNodeAccountMapping(acc)"
              >强制解绑</el-button>
            </template>
          </el-table-column>
        </el-table>
        <div class="section-title-wrap mt10">
          <span class="section-icon tone-dup"><el-icon><List /></el-icon></span>
          <div class="title">解绑记录</div>
        </div>
        <el-table :data="profileUnbindRecords" stripe max-height="220" empty-text="暂无解绑记录">
          <el-table-column label="来源" width="120">
            <template #default="{ row }">{{ unbindSourceText(row.source_type) }}</template>
          </el-table-column>
          <el-table-column prop="node_id" label="节点编号" width="140" />
          <el-table-column prop="local_username" label="节点账号" width="140" />
          <el-table-column label="状态" width="120">
            <template #default="{ row }">
              <el-tag size="small" :type="unbindStatusTagType(row.status)">{{ unbindStatusText(row.status) }}</el-tag>
            </template>
          </el-table-column>
          <el-table-column prop="initiated_by" label="发起人" width="120" />
          <el-table-column prop="reason" label="理由" min-width="220" show-overflow-tooltip />
          <el-table-column label="更新时间" min-width="180">
            <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
          </el-table-column>
        </el-table>
      </template>
    </el-dialog>

    <el-dialog v-model="duplicatesVisible" width="860px">
      <template #header>
        <div class="dialog-title-wrap">
          <span class="section-icon tone-dup"><el-icon><WarningFilled /></el-icon></span>
          <span class="title">重复身份查询结果</span>
        </div>
      </template>
      <el-alert
        title="仅在点击“查重”时发起查询，平时不查询。"
        type="info"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-skeleton v-if="duplicatesLoading" :rows="4" animated />
      <template v-else>
        <div class="section-title-wrap">
          <span class="section-icon tone-active"><el-icon><UserFilled /></el-icon></span>
          <div class="title">当前在用平台账号</div>
        </div>
        <el-table :data="duplicateActiveRows" stripe max-height="220" empty-text="暂无数据">
          <el-table-column prop="username" label="平台账号" width="150" />
          <el-table-column label="平台UID" width="110">
            <template #default="{ row }">{{ row.platform_uid ?? "-" }}</template>
          </el-table-column>
          <el-table-column prop="real_name" label="真实姓名" width="120" />
          <el-table-column prop="student_id" label="学号" width="140" />
          <el-table-column prop="email" label="邮箱" min-width="220" />
        </el-table>
        <div class="section-title-wrap mt10">
          <span class="section-icon tone-delete"><el-icon><Delete /></el-icon></span>
          <div class="title">已删除平台账号</div>
        </div>
        <el-table :data="duplicateDeletedRows" stripe max-height="220" empty-text="暂无数据">
          <el-table-column prop="deleted_id" label="删除ID" width="90" />
          <el-table-column prop="username" label="平台账号" width="150" />
          <el-table-column label="平台UID" width="110">
            <template #default="{ row }">{{ row.platform_uid ?? "-" }}</template>
          </el-table-column>
          <el-table-column prop="real_name" label="真实姓名" width="120" />
          <el-table-column prop="student_id" label="学号" width="140" />
          <el-table-column prop="email" label="邮箱" min-width="220" />
          <el-table-column label="删除时间" width="180">
            <template #default="{ row }">{{ fmtTime(row.deleted_at) }}</template>
          </el-table-column>
          <el-table-column label="UID释放" min-width="220">
            <template #default="{ row }">{{ fmtUIDRelease(row) }}</template>
          </el-table-column>
        </el-table>
      </template>
    </el-dialog>
  </el-card>
</template>

<script setup lang="ts">
import { computed, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import { ApiClient, type AdminUserDetail, type DeletedUserAccount, type GraduationDueUser, type UserNodeAccount, type UserNodeUnbindRecord, type UserProfile } from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime } from "../../lib/time";
import { Bell, Check, Connection, Delete, List, UserFilled, WarningFilled } from "@element-plus/icons-vue";

const loading = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<AdminUserDetail[]>([]);
const deletedRows = ref<DeletedUserAccount[]>([]);
const blacklistSet = ref<Set<string>>(new Set());
const blacklistKeySet = ref<Set<string>>(new Set());
const whitelistSet = ref<Set<string>>(new Set());
const exemptionSet = ref<Set<string>>(new Set());
const keyword = ref("");
type UserKeywordField = "all" | "username" | "platform_uid" | "real_name" | "student_id" | "advisor" | "email" | "phone" | "role" | "status";
type UserSortOrder = "ascending" | "descending" | null;
type UserSortKey = "username" | "platform_uid" | "real_name" | "role" | "two_factor_enabled" | "student_id" | "email" | "expected_graduation_year" | "balance" | "status";
const keywordField = ref<UserKeywordField>("all");
const userSortKey = ref<UserSortKey | "">("");
const userSortOrder = ref<UserSortOrder>(null);
const keywordPlaceholder = computed(() => {
  if (keywordField.value === "username") return "输入平台账号关键词";
  if (keywordField.value === "platform_uid") return "输入平台UID关键词";
  if (keywordField.value === "real_name") return "输入真实姓名关键词";
  if (keywordField.value === "student_id") return "输入学号关键词";
  if (keywordField.value === "advisor") return "输入导师关键词";
  if (keywordField.value === "email") return "输入邮箱关键词";
  if (keywordField.value === "phone") return "输入手机号关键词";
  if (keywordField.value === "role") return "输入角色关键词";
  if (keywordField.value === "status") return "输入状态关键词";
  return "平台账号 / UID / 姓名 / 学号 / 导师 / 邮箱 / 手机 / 角色 / 状态";
});
const filteredRows = computed(() => {
  const source = rows.value || [];
  const k = keyword.value.trim().toLowerCase();
  const filterActive = k !== "";
  const matched = source.filter((row) => {
    if (filterActive && isAdminRole(row.role)) return false;
    if (!filterActive) return true;
    return matchesAdminUserKeyword(row, k, keywordField.value);
  });
  if (filterActive) {
    return sortAdminUsers(matched, userSortKey.value, userSortOrder.value);
  }
  const admins = matched.filter((row) => isAdminRole(row.role));
  const others = matched.filter((row) => !isAdminRole(row.role));
  return [
    ...sortAdminUsers(admins, userSortKey.value, userSortOrder.value),
    ...sortAdminUsers(others, userSortKey.value, userSortOrder.value),
  ];
});

const rechargeVisible = ref(false);
const rechargeLoading = ref(false);
const rechargeUser = ref("");
const rechargeAmount = ref(100);
const rechargeMethod = ref("admin");
const rechargeGeneralBalance = ref(0);
const rechargeCarryoverBalance = ref(0);
const rechargeExclusiveBalance = ref(0);
const rechargeTotalBalance = ref(0);
const rechargeBalanceLoading = ref(false);
const rechargeBalanceError = ref("");
const dueLoading = ref(false);
const sendLoading = ref(false);
const dueDeleteLoading = ref(false);
const profileVisible = ref(false);
const profileLoading = ref(false);
const profileError = ref("");
const profileData = ref<{
  username: string;
  platform_uid?: number;
  email: string;
  real_name: string;
  student_id: string;
  advisor: string;
  expected_graduation_year: number;
  expected_graduation_month: number;
  phone: string;
  role: string;
  two_factor_enabled?: boolean;
  balance: number;
  general_balance?: number;
  carryover_balance?: number;
  exclusive_balance?: number;
  total_balance?: number;
  status: string;
  node_accounts: UserNodeAccount[];
} | null>(null);
const profileUnbindRecords = ref<UserNodeUnbindRecord[]>([]);
const duplicatesVisible = ref(false);
const duplicatesLoading = ref(false);
const duplicateActiveRows = ref<UserProfile[]>([]);
const duplicateDeletedRows = ref<DeletedUserAccount[]>([]);
type GraduationDueView = GraduationDueUser & { send_success?: boolean; send_error?: string };
const dueRows = ref<GraduationDueView[]>([]);
const selectedDueRows = ref<GraduationDueView[]>([]);

function client() {
  return new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
}

function fmt2(v: number): string {
  return Number(v ?? 0).toFixed(2);
}

function isAdminRole(role?: string): boolean {
  return String(role || "").trim() === "admin";
}

function compareNumberLike(a: number, b: number): number {
  return a === b ? 0 : (a < b ? -1 : 1);
}

function compareTextLike(a: string, b: string): number {
  return a.localeCompare(b, "zh-Hans-CN", { numeric: true, sensitivity: "base" });
}

function adminRoleWeight(role?: string): number {
  const value = String(role || "").trim();
  if (value === "admin") return 0;
  if (value === "power_user") return 1;
  return 2;
}

function adminUserFieldValue(row: AdminUserDetail, field: UserKeywordField): string {
  if (field === "username") return String(row.username || "").trim();
  if (field === "platform_uid") return String(row.platform_uid ?? "").trim();
  if (field === "real_name") return String(row.real_name || "").trim();
  if (field === "student_id") return String(row.student_id || "").trim();
  if (field === "advisor") return String(row.advisor || "").trim();
  if (field === "email") return String(row.email || "").trim();
  if (field === "phone") return String(row.phone || "").trim();
  if (field === "role") return `${String(row.role || "").trim()} ${roleText(row.role)}`.trim();
  if (field === "status") return `${String(row.status || "").trim()} ${effectiveStatus(row)}`.trim();
  return [
    row.username,
    row.platform_uid,
    row.real_name,
    row.student_id,
    row.advisor,
    row.email,
    row.phone,
    row.role,
    roleText(row.role),
    row.status,
    effectiveStatus(row),
  ].map((value) => String(value ?? "").trim()).join(" ");
}

function matchesAdminUserKeyword(row: AdminUserDetail, keywordText: string, field: UserKeywordField): boolean {
  return adminUserFieldValue(row, field).toLowerCase().includes(keywordText);
}

function adminUserSortValue(row: AdminUserDetail, key: UserSortKey): number | string {
  if (key === "platform_uid") return Number(row.platform_uid ?? 0);
  if (key === "role") return adminRoleWeight(row.role);
  if (key === "two_factor_enabled") return row.two_factor_enabled ? 1 : 0;
  if (key === "expected_graduation_year") {
    const year = Number(row.expected_graduation_year ?? 0);
    const month = Number(row.expected_graduation_month ?? 0);
    return year * 100 + month;
  }
  if (key === "balance") return Number(row.balance ?? 0);
  if (key === "status") return effectiveStatus(row);
  return String((row as any)?.[key] ?? "").trim();
}

function sortAdminUsers(items: AdminUserDetail[], key: UserSortKey | "", order: UserSortOrder): AdminUserDetail[] {
  const base = Array.isArray(items) ? [...items] : [];
  if (!key || !order) return base;
  const direction = order === "descending" ? -1 : 1;
  return base
    .map((row, index) => ({ row, index }))
    .sort((left, right) => {
      const a = adminUserSortValue(left.row, key);
      const b = adminUserSortValue(right.row, key);
      const delta = typeof a === "number" && typeof b === "number"
        ? compareNumberLike(a, b)
        : compareTextLike(String(a), String(b));
      if (delta !== 0) return delta * direction;
      return left.index - right.index;
    })
    .map((entry) => entry.row);
}

function setRechargeBalances(generalBalance: number, carryoverBalance: number, exclusiveBalance: number, totalBalance?: number) {
  const g = Number(generalBalance || 0);
  const c = Number(carryoverBalance || 0);
  const e = Number(exclusiveBalance || 0);
  rechargeGeneralBalance.value = g;
  rechargeCarryoverBalance.value = c;
  rechargeExclusiveBalance.value = e;
  rechargeTotalBalance.value = Number(totalBalance ?? (g + c + e));
}

function roleText(role: string): string {
  if (role === "admin") return "管理员";
  if (role === "power_user") return "高级用户";
  return "普通用户";
}

function pushIdentity(set: Set<string>, v: string): void {
  const x = String(v || "").trim();
  if (x) set.add(x);
}

function buildIdentitySet(entries: Array<{
  local_username?: string;
  billing_username?: string;
  source_platform_username?: string;
}>): Set<string> {
  const out = new Set<string>();
  for (const e of entries ?? []) {
    const sourcePlatform = String(e?.source_platform_username || "").trim();
    const billing = String(e?.billing_username || "").trim();
    if (sourcePlatform) {
      pushIdentity(out, sourcePlatform);
      continue;
    }
    if (billing) {
      pushIdentity(out, billing);
    }
  }
  return out;
}

function isBlack(row: AdminUserDetail): boolean {
  const u = String(row.username || "").trim();
  return blacklistSet.value.has(u);
}

function isWhite(row: AdminUserDetail): boolean {
  const u = String(row.username || "").trim();
  return whitelistSet.value.has(u);
}

function isExempt(row: AdminUserDetail): boolean {
  const u = String(row.username || "").trim();
  return exemptionSet.value.has(u);
}

function accountBlacklistKey(nodeID: string, localUsername: string): string {
  return `${String(nodeID || "").trim()}|${String(localUsername || "").trim()}`;
}

function isNodeAccountBlack(nodeID: string, localUsername: string): boolean {
  const node = String(nodeID || "").trim();
  const local = String(localUsername || "").trim();
  if (!node || !local) return false;
  return blacklistKeySet.value.has(accountBlacklistKey(node, local)) || blacklistKeySet.value.has(accountBlacklistKey("*", local));
}

function effectiveStatus(row: AdminUserDetail): string {
  const current = String(row.status || "").trim();
  if (isBlack(row)) return "blocked";
  if (current === "blocked") return "normal";
  return current || "normal";
}

function fmtGrad(year: number, month: number): string {
  if (!year || !month) return "-";
  return `${year}-${String(month).padStart(2, "0")}`;
}

function fmtTime(v: string): string {
  return formatServerDateTime(v);
}

function mappingStateTip(row: UserNodeAccount): string {
  if (row.identity_initializing) return "正在同步 UID/GID，完成前无法 SSH 登录";
  if (!row.identity_aligned) return "节点尚未回传最新 UID/GID 快照，请稍后自动刷新";
  return "";
}

function fmtDurationFromSeconds(seconds?: number): string {
  const total = Number(seconds ?? 0);
  if (!Number.isFinite(total) || total <= 0) return "已可释放";
  const days = Math.floor(total / 86400);
  const years = Math.floor(days / 365);
  const remDaysAfterYears = days % 365;
  const months = Math.floor(remDaysAfterYears / 30);
  const remDays = remDaysAfterYears % 30;
  const hours = Math.floor((total % 86400) / 3600);
  const parts: string[] = [];
  if (years > 0) parts.push(`${years}年`);
  if (months > 0) parts.push(`${months}个月`);
  if (remDays > 0) parts.push(`${remDays}天`);
  if (parts.length === 0) parts.push(`${hours}小时`);
  return parts.slice(0, 3).join("");
}

function fmtUIDRelease(row: DeletedUserAccount): string {
  const uid = Number(row.platform_uid ?? 0);
  if (!Number.isFinite(uid) || uid <= 0) return "-";
  const releaseAt = String(row.uid_release_at || "").trim();
  const remaining = Number(row.uid_release_remaining_seconds ?? NaN);
  if (releaseAt) {
    return `${fmtDurationFromSeconds(remaining)}（至 ${fmtTime(releaseAt)}）`;
  }
  const deletedAt = String(row.deleted_at || "").trim();
  if (!deletedAt) return "-";
  const release = new Date(deletedAt);
  if (Number.isNaN(release.getTime())) return "-";
  release.setFullYear(release.getFullYear() + 3);
  const remainingSeconds = Math.max(0, Math.floor((release.getTime() - Date.now()) / 1000));
  return `${fmtDurationFromSeconds(remainingSeconds)}（至 ${fmtTime(release.toISOString())}）`;
}

function unbindSourceText(sourceType: string): string {
  return String(sourceType || "").trim() === "admin_forced" ? "管理员强制" : "用户申请";
}

function unbindStatusText(status: string): string {
  const s = String(status || "").trim();
  if (s === "forced") return "已强制解绑";
  if (s === "approved") return "已审批解绑";
  if (s === "rejected") return "已驳回";
  return "待审批";
}

function unbindStatusTagType(status: string): "success" | "danger" | "warning" | "info" {
  const s = String(status || "").trim();
  if (s === "approved") return "success";
  if (s === "forced") return "danger";
  if (s === "rejected") return "info";
  return "warning";
}

async function reload() {
  loading.value = true;
  error.value = "";
  success.value = "";
  rows.value = [];
  try {
    const r1 = await client().adminUsersDetails(2000);
    rows.value = r1.users ?? [];
    try {
      const r2 = await client().adminDeletedUsers(2000, false);
      deletedRows.value = r2.users ?? [];
    } catch (e: any) {
      // 兼容旧后端：未提供删除恢复接口时不阻塞主页面展示。
      if (e?.status === 404) {
        deletedRows.value = [];
      } else {
        throw e;
      }
    }
    try {
      const [bl, wl, ex] = await Promise.all([
        client().adminBlacklist(""),
        client().adminWhitelist(""),
        client().adminExemptions(""),
      ]);
      blacklistKeySet.value = new Set((bl.entries ?? []).map((x) => accountBlacklistKey(x.node_id, x.local_username)).filter((x) => x !== "|"));
      blacklistSet.value = buildIdentitySet(bl.entries ?? []);
      whitelistSet.value = buildIdentitySet(wl.entries ?? []);
      exemptionSet.value = buildIdentitySet(ex.entries ?? []);
    } catch {
      blacklistKeySet.value = new Set();
      blacklistSet.value = new Set();
      whitelistSet.value = new Set();
      exemptionSet.value = new Set();
    }
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function syncRechargeCurrentBalance(username: string) {
  const u = String(username || "").trim();
  if (!u) return;
  rechargeBalanceLoading.value = true;
  rechargeBalanceError.value = "";
  try {
    const r = await client().adminPlatformUserDetail(u);
    if (String(rechargeUser.value || "").trim() !== u) return;
    const user = r.user;
    const general = Number(user.general_balance ?? user.balance ?? 0);
    const carryover = Number(user.carryover_balance ?? 0);
    const exclusive = Number(user.exclusive_balance ?? 0);
    const total = Number(user.total_balance ?? (general + carryover + exclusive));
    setRechargeBalances(general, carryover, exclusive, total);
  } catch (e: any) {
    if (String(rechargeUser.value || "").trim() !== u) return;
    rechargeBalanceError.value = `同步当前积分失败：${e?.message ?? String(e)}`;
  } finally {
    if (String(rechargeUser.value || "").trim() === u) {
      rechargeBalanceLoading.value = false;
    }
  }
}

function openRecharge(row: AdminUserDetail) {
  rechargeUser.value = String(row.username || "").trim();
  rechargeAmount.value = 100;
  rechargeMethod.value = "admin";
  rechargeBalanceError.value = "";
  rechargeBalanceLoading.value = false;
  const general = Number(row.balance ?? 0);
  const carryover = Number(row.carryover_balance ?? 0);
  const exclusive = Number(row.exclusive_balance ?? 0);
  const total = Number(row.total_balance ?? (general + carryover + exclusive));
  setRechargeBalances(general, carryover, exclusive, total);
  rechargeVisible.value = true;
  void syncRechargeCurrentBalance(rechargeUser.value);
}

async function openProfile(username: string) {
  const u = String(username || "").trim();
  if (!u) return;
  profileVisible.value = true;
  profileLoading.value = true;
  profileError.value = "";
  profileData.value = null;
  profileUnbindRecords.value = [];
  try {
    const [detail, unbindRecordsResp] = await Promise.all([
      client().adminPlatformUserDetail(u),
      (async () => {
        try {
          return await client().adminUnbindRecords({ billing_username: u, limit: 200 });
        } catch (e: any) {
          if (e?.status === 404) {
            return { records: [] as UserNodeUnbindRecord[] };
          }
          throw e;
        }
      })(),
    ]);
    profileData.value = detail.user;
    profileUnbindRecords.value = unbindRecordsResp.records ?? [];
  } catch (e: any) {
    profileError.value = e?.message ?? String(e);
  } finally {
    profileLoading.value = false;
  }
}

async function submitProfileUnbindRequest(acc: { node_id: string; local_username: string }) {
  const billing = String(profileData.value?.username || "").trim();
  const nodeID = String(acc?.node_id || "").trim();
  const local = String(acc?.local_username || "").trim();
  if (!billing || !nodeID || !local) {
    error.value = "提交解绑申请失败：映射信息不完整";
    return;
  }
  let reason = "";
  try {
    const promptRes = await ElMessageBox.prompt(
      `请填写代提交解绑申请理由（至少 10 个字）：\n平台账号：${billing}\n节点编号：${nodeID}\n节点账号：${local}`,
      "代提交解绑申请",
      {
        type: "warning",
        confirmButtonText: "下一步",
        cancelButtonText: "取消",
        inputPlaceholder: "例如：管理员核查确认停用该映射",
      },
    );
    reason = String((promptRes as any)?.value || "").trim();
    if (reason.length < 10) {
      ElMessage.warning("解绑理由至少 10 个字");
      return;
    }
  } catch {
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确认代平台账号 ${billing} 提交解绑申请吗？\n节点编号：${nodeID}\n节点账号：${local}`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认提交", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await client().adminCreateUnbindRequest({
      billing_username: billing,
      node_id: nodeID,
      local_username: local,
      reason,
    });
    success.value = `已代提交解绑申请：${nodeID}/${local}`;
    await openProfile(billing);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function forceUnbindNodeAccountMapping(acc: { node_id: string; local_username: string }) {
  const billing = String(profileData.value?.username || "").trim();
  const nodeID = String(acc?.node_id || "").trim();
  const local = String(acc?.local_username || "").trim();
  if (!billing || !nodeID || !local) {
    error.value = "强制解绑失败：映射信息不完整";
    return;
  }
  let reason = "";
  try {
    const promptRes = await ElMessageBox.prompt(
      `请填写强制解绑理由（至少 10 个字）：\n平台账号：${billing}\n节点编号：${nodeID}\n节点账号：${local}`,
      "强制解绑",
      {
        type: "warning",
        confirmButtonText: "下一步",
        cancelButtonText: "取消",
        inputPlaceholder: "例如：违规使用、账号停用、归属变更",
      },
    );
    reason = String((promptRes as any)?.value || "").trim();
    if (reason.length < 10) {
      ElMessage.warning("解绑理由至少 10 个字");
      return;
    }
  } catch {
    return;
  }
  try {
    await ElMessageBox.confirm(
      `你正在执行强制解绑：\n平台账号：${billing}\n节点编号：${nodeID}\n节点账号：${local}\n\n该操作会立即终止进程并断开 SSH。`,
      "第一次确认",
      { type: "warning", confirmButtonText: "继续", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await ElMessageBox.confirm(
      `最后确认：是否强制解绑 ${nodeID}/${local} ？\n\n理由：${reason}`,
      "第二次确认",
      { type: "warning", confirmButtonText: "确认强制解绑", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await client().adminDeleteAccount({
      billing_username: billing,
      node_id: nodeID,
      local_username: local,
      reason,
    });
    success.value = `已强制解绑：${nodeID}/${local}`;
    await Promise.all([openProfile(billing), reload()]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function disableNodeAccountMapping(acc: { node_id: string; local_username: string }) {
  const billing = String(profileData.value?.username || "").trim();
  const nodeID = String(acc?.node_id || "").trim();
  const local = String(acc?.local_username || "").trim();
  if (!billing || !nodeID || !local) {
    error.value = "禁用失败：映射信息不完整";
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确认禁用该节点账号映射吗？\n平台账号：${billing}\n节点编号：${nodeID}\n节点账号：${local}\n\n禁用后将无法 SSH 登录，并会强制下线且终止该账号当前全部进程。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认禁用", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  let reason = "";
  try {
    const promptRes = await ElMessageBox.prompt("请输入禁用理由（可留空）", "禁用理由", {
      confirmButtonText: "确认",
      cancelButtonText: "取消",
      inputPlaceholder: "默认空",
    });
    reason = String((promptRes as any)?.value || "").trim();
  } catch {
    return;
  }
  try {
    await client().adminDisableAccountMapping({
      billing_username: billing,
      node_id: nodeID,
      local_username: local,
      reason,
    });
    success.value = `已禁用映射：${nodeID}/${local}（已下发强制下线与进程终止）`;
    await openProfile(billing);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function enableNodeAccountMapping(acc: { node_id: string; local_username: string }) {
  const billing = String(profileData.value?.username || "").trim();
  const nodeID = String(acc?.node_id || "").trim();
  const local = String(acc?.local_username || "").trim();
  if (!billing || !nodeID || !local) {
    error.value = "解除禁用失败：映射信息不完整";
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确认解除该节点账号映射的禁用吗？\n平台账号：${billing}\n节点编号：${nodeID}\n节点账号：${local}`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认解除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  let reason = "";
  try {
    const promptRes = await ElMessageBox.prompt("请输入解除禁用理由（可留空）", "解除禁用理由", {
      confirmButtonText: "确认",
      cancelButtonText: "取消",
      inputPlaceholder: "默认空",
    });
    reason = String((promptRes as any)?.value || "").trim();
  } catch {
    return;
  }
  try {
    try {
      await client().adminEnableAccountMapping({
        billing_username: billing,
        node_id: nodeID,
        local_username: local,
        reason,
      });
    } catch (e: any) {
      if (e?.status === 404) {
        await client().adminDeleteBlacklist(nodeID, local);
      } else {
        throw e;
      }
    }
    success.value = `已解除禁用映射：${nodeID}/${local}`;
    await openProfile(billing);
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function blockUser(row: AdminUserDetail) {
  if (isBlack(row)) {
    ElMessage.warning(`账号 ${row.username} 已在黑名单`);
    return;
  }
  let reason = "";
  try {
    await ElMessageBox.confirm(`确认拉黑平台账号 ${row.username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认拉黑", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    const promptRes = await ElMessageBox.prompt("请输入拉黑理由（可留空）", "拉黑理由", {
      confirmButtonText: "确认",
      cancelButtonText: "取消",
      inputPlaceholder: "默认空",
    });
    reason = String((promptRes as any)?.value || "").trim();
  } catch {
    return;
  }
  try {
    try {
      await client().adminBlockUser(row.username, reason);
    } catch (e: any) {
      // 兼容：旧后端没有 /users/:username/block 时，直接写 SSH 黑名单（全节点）。
      if (e?.status === 404) {
        await client().adminUpsertBlacklist("*", [], [row.username], reason);
      } else {
        throw e;
      }
    }
    success.value = `已拉黑：${row.username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function unblockUser(row: AdminUserDetail) {
  if (!isBlack(row)) {
    ElMessage.warning(`账号 ${row.username} 当前未拉黑`);
    return;
  }
  try {
    await ElMessageBox.confirm(`确认解黑平台账号 ${row.username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认解黑", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    try {
      await client().adminUnblockUser(row.username);
    } catch (e: any) {
      if (e?.status !== 404) throw e;
    }
    // 解黑时同步尝试从全局 SSH 黑名单移除
    try {
      await client().adminDeleteBlacklist("*", row.username);
    } catch (e: any) {
      if (e?.status !== 404) throw e;
    }
    success.value = `已解黑：${row.username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function deleteUser(row: AdminUserDetail) {
  try {
    await ElMessageBox.confirm(`确认删除平台账号 ${row.username} 吗？删除后可在“已删除平台账号”中恢复，平台 UID 将在删除满 3 年后释放。`, "二次确认", { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    await client().adminDeleteUserCompat(row.username, "管理员手动删除");
    success.value = `已删除：${row.username}`;
    await reload();
  } catch (e: any) {
    if (e?.status === 404 && String(e?.message || "").trim() === "请求的资源不存在") {
      const base = (settingsState.baseUrl || window.location.origin || "").trim() || window.location.origin;
      error.value = `删除接口不可用：当前控制器地址为 ${base}，请确认该实例已更新并重启。`;
    } else {
      error.value = e?.message ?? String(e);
    }
  }
}

async function restoreUser(row: DeletedUserAccount) {
  try {
    await ElMessageBox.confirm(`确认恢复平台账号 ${row.username} 吗？`, "二次确认", { type: "warning", confirmButtonText: "确认恢复", cancelButtonText: "取消" });
  } catch {
    return;
  }
  try {
    await client().adminRestoreDeletedUser(row.deleted_id);
    success.value = `已恢复：${row.username}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function queryDuplicates(row: AdminUserDetail) {
  duplicatesVisible.value = true;
  duplicatesLoading.value = true;
  duplicateActiveRows.value = [];
  duplicateDeletedRows.value = [];
  try {
    const r = await client().adminFindUserDuplicates({
      username: row.username,
      email: row.email,
      student_id: row.student_id,
      limit: 200,
    });
    duplicateActiveRows.value = r.active_users ?? [];
    duplicateDeletedRows.value = r.deleted_users ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    duplicatesLoading.value = false;
  }
}

async function doRecharge() {
  if (!rechargeAmount.value) {
    error.value = "调整值不能为 0";
    return;
  }
  rechargeLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminRecharge(rechargeUser.value, rechargeAmount.value, rechargeMethod.value);
    const general = Number(r.general_balance ?? r.balance ?? rechargeGeneralBalance.value);
    const carryover = Number(r.carryover_balance ?? rechargeCarryoverBalance.value);
    const exclusive = Number(r.exclusive_balance ?? rechargeExclusiveBalance.value);
    const total = Number(r.total_balance ?? (general + carryover + exclusive));
    setRechargeBalances(general, carryover, exclusive, total);
    rechargeVisible.value = false;
    success.value = `积分调整成功，当前通用积分 ${fmt2(general)}、结转积分 ${fmt2(carryover)}，总积分 ${fmt2(total)}`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    rechargeLoading.value = false;
  }
}

async function loadGraduationDueUsers() {
  dueLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminGraduationDueUsers(5000);
    dueRows.value = (r.users ?? []).map((x) => ({ ...x }));
    selectedDueRows.value = [];
    success.value = `查询完成：共 ${dueRows.value.length} 人已到期`;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    dueLoading.value = false;
  }
}

function onDueSelectionChange(rows: GraduationDueView[]) {
  selectedDueRows.value = rows ?? [];
}

function onAdminUserSortChange({ prop, order }: { prop: string; order: UserSortOrder }) {
  if (!prop || !order) {
    userSortKey.value = "";
    userSortOrder.value = null;
    return;
  }
  userSortKey.value = prop as UserSortKey;
  userSortOrder.value = order;
}

async function sendReminder(usernames: string[]) {
  if (usernames.length === 0) {
    ElMessage.warning("请先选择用户");
    return;
  }
  sendLoading.value = true;
  error.value = "";
  success.value = "";
  try {
    const r = await client().adminSendGraduationReminders(usernames);
    const m = new Map((r.results ?? []).map((x) => [x.username, x]));
    dueRows.value = dueRows.value.map((row) => {
      const rr = m.get(row.username);
      if (!rr) return row;
      return {
        ...row,
        send_success: rr.success,
        send_error: rr.success ? "" : (rr.error || "发送失败"),
      };
    });
    success.value = `发送完成：成功 ${r.success}，失败 ${r.failed}`;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    sendLoading.value = false;
  }
}

async function sendSelectedDueUsers() {
  await sendReminder(selectedDueRows.value.map((x) => x.username));
}

async function sendAllDueUsers() {
  await sendReminder(dueRows.value.map((x) => x.username));
}

async function sendSingleDueUser(row: GraduationDueView) {
  await sendReminder([row.username]);
}

async function deleteDueUsers(usernames: string[]) {
  const targets = Array.from(new Set((usernames ?? []).map((x) => String(x || "").trim()).filter(Boolean)));
  if (targets.length === 0) {
    ElMessage.warning("请先勾选要删除的到期用户");
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确认删除已勾选的 ${targets.length} 个到期平台账号吗？\n删除后可在“已删除平台账号（可恢复）”中恢复。`,
      "批量删除确认",
      { type: "warning", confirmButtonText: "确认删除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  dueDeleteLoading.value = true;
  error.value = "";
  success.value = "";
  const failed: string[] = [];
  const successUsers = new Set<string>();
  try {
    for (const username of targets) {
      try {
        await client().adminDeleteUserCompat(username, "毕业到期批量删除");
        successUsers.add(username);
      } catch (e: any) {
        failed.push(`${username}：${e?.message ?? "删除失败"}`);
      }
    }
    if (successUsers.size > 0) {
      dueRows.value = dueRows.value.filter((row) => !successUsers.has(String(row.username || "").trim()));
      selectedDueRows.value = selectedDueRows.value.filter((row) => !successUsers.has(String(row.username || "").trim()));
      await reload();
    }
    success.value = `毕业到期账号删除完成：成功 ${successUsers.size}，失败 ${failed.length}`;
    if (failed.length > 0) {
      error.value = failed.slice(0, 5).join("\n");
    }
  } finally {
    dueDeleteLoading.value = false;
  }
}

async function deleteSelectedDueUsers() {
  await deleteDueUsers(selectedDueRows.value.map((x) => x.username));
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
  width: 28px;
  height: 28px;
  border-radius: 8px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  color: #dbeafe;
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  flex-shrink: 0;
}
.tone-delete {
  background: linear-gradient(135deg, #991b1b, #dc2626);
  color: #fee2e2;
}
.tone-list {
  background: linear-gradient(135deg, #0f766e, #14b8a6);
  color: #ccfbf1;
}
.tone-profile {
  background: linear-gradient(135deg, #1e3a8a, #2563eb);
  color: #dbeafe;
}
.tone-map {
  background: linear-gradient(135deg, #312e81, #4f46e5);
  color: #e0e7ff;
}
.tone-dup {
  background: linear-gradient(135deg, #9a3412, #ea580c);
  color: #ffedd5;
}
.tone-active {
  background: linear-gradient(135deg, #166534, #16a34a);
  color: #dcfce7;
}
.tone-remind {
  background: linear-gradient(135deg, #a16207, #d97706);
  color: #fef3c7;
}
.dialog-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
}
.mt10 {
  margin-top: 10px;
}
.sub {
  margin-top: 4px;
  font-size: 12px;
  color: #6b7280;
}
.recharge-balance-wrap {
  display: inline-flex;
  align-items: center;
  gap: 8px;
  flex-wrap: wrap;
}
.recharge-balance-error {
  color: var(--error-color);
}
.expand-wrap {
  padding: 8px 12px;
}
.mb {
  margin-bottom: 10px;
}
.mapping-state-cell {
  display: flex;
  flex-direction: column;
  gap: 6px;
}
.mapping-state-tip {
  font-size: 12px;
  line-height: 1.4;
  color: #64748b;
}
.username-link {
  padding: 0;
  border: none;
  background: transparent;
  color: #16a34a;
  font-weight: 600;
  cursor: pointer;
}
.username-link:hover {
  text-decoration: underline;
}
.username-cell {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.black-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: #dc2626;
}
.white-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: #2563eb;
}
.exempt-badge {
  display: inline-flex;
  align-items: center;
  justify-content: center;
  width: 18px;
  height: 18px;
  border-radius: 999px;
  font-size: 12px;
  font-weight: 700;
  color: #fff;
  background: #7c3aed;
}

@media (max-width: 920px) {
  .row {
    flex-wrap: wrap;
    align-items: flex-start;
  }
  .section-title-wrap {
    width: 100%;
  }
  .content-stack :deep(.el-form--inline .el-form-item) {
    margin-right: 8px;
    margin-bottom: 8px;
  }
}
</style>
