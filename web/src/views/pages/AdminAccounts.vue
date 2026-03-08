<template>
  <div class="admin-accounts-page">
    <el-card class="section-card overview-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-map"><el-icon><Connection /></el-icon></span>
            <span>平台账号映射管理（管理员）</span>
          </div>
          <div class="head-actions">
            <el-button :loading="loading" @click="reload">刷新</el-button>
            <el-button type="primary" @click="openCreateDialog">新增映射</el-button>
          </div>
        </div>
      </template>

      <el-alert v-if="error" :title="error" type="error" show-icon class="mb" />
      <el-alert v-if="success" :title="success" type="success" show-icon class="mb" />
      <el-alert title="同一平台账号可映射多个节点账号；唯一键是“节点编号 + 节点账号”。" type="info" show-icon class="mb" />
      <el-alert
        title="页面阅读建议：先做“映射查询”定位账号，再在“映射列表”维护映射，随后处理“解绑记录”和“受限名单”。节点账号开通相关内容已单独移到“节点账号开通”页面。"
        type="warning"
        :closable="false"
        show-icon
      />
    </el-card>

    <el-card class="section-card bind-policy-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-security"><el-icon><Clock /></el-icon></span>
            <span>绑定挑战临时窗口策略</span>
          </div>
          <el-button type="primary" size="small" :loading="bindPolicySaving" @click="saveBindPolicy">保存策略</el-button>
        </div>
      </template>
      <el-skeleton v-if="bindPolicyLoading" :rows="2" animated />
      <template v-else>
        <el-alert
          title="用于控制用户执行 gpuops-claim 前的 5 分钟临时窗口：可登录、但限 CPU/内存并隐藏 GPU。失败后将自动强制下线与清进程。"
          type="warning"
          :closable="false"
          show-icon
          class="mb"
        />
        <el-form label-position="top">
          <el-row :gutter="12">
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>挑战窗口（秒）</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>提交绑定后，用户必须在该时长内执行 <code>gpuops-claim</code> 完成校验，超时则判定失败。</div>
                          <div>示例：300 = 5 分钟。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-input-number v-model="bindPolicy.challenge_window_seconds" :min="60" :max="1800" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>临时 CPU 限速（%）</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>挑战窗口生效期间，对目标节点账号下发 CPU 配额上限。</div>
                          <div>示例：20 = 挑战期最多使用约 20% CPU 配额。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-input-number v-model="bindPolicy.trial_cpu_quota_percent" :min="1" :max="100" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>临时内存上限（GB）</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>挑战窗口内对目标节点账号设置内存上限，校验通过后恢复常态。</div>
                          <div>示例：8 = 挑战期最多用 8GB 内存。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-input-number v-model="bindPolicy.trial_memory_limit_gb" :min="0.5" :max="1024" :step="0.5" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12">
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>首次失败冷却（分钟）</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>同一平台账号第一次挑战失败后，需要等待该时长才能再次发起绑定挑战。</div>
                          <div>示例：30 = 首次失败后 30 分钟内不能再申请。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-input-number v-model="bindPolicy.first_failure_cooldown_minutes" :min="1" :max="1440" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>重复失败冷却（分钟）</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>同一平台账号连续失败（第 2 次及以后）时使用该冷却时长，惩罚会更重。</div>
                          <div>示例：180 = 连续失败后需等待 3 小时。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-input-number v-model="bindPolicy.repeat_failure_cooldown_minutes" :min="1" :max="4320" style="width: 100%" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>先到先得（历史参数）</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>当前策略为“先到先得”：同一目标账号已有进行中的 challenge 时，后续申请会直接失败。</div>
                          <div>该参数目前不参与生效判定，保留仅为兼容历史配置。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-input-number v-model="bindPolicy.contention_freeze_minutes" :min="1" :max="1440" style="width: 100%" />
              </el-form-item>
            </el-col>
          </el-row>
          <el-row :gutter="12">
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>临时隐藏 GPU</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>开启后，挑战窗口内会对目标账号下发隐藏 GPU；校验成功后自动恢复。</div>
                          <div>示例：开启 = 只能先做 CPU/SSH 校验，避免挑战期直接占用 GPU。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-switch v-model="bindPolicy.trial_gpu_blocked" />
              </el-form-item>
            </el-col>
            <el-col :span="8">
              <el-form-item>
                <template #label>
                  <span class="policy-label">
                    <span>同账号仅一个挑战窗口</span>
                    <el-tooltip placement="top">
                      <template #content>
                        <div class="policy-help-tip">
                          <div>开启后，同一平台账号同一时间只能存在一个活跃 challenge。</div>
                          <div>已有 challenge 未结束时，不能再发起其他绑定。</div>
                        </div>
                      </template>
                      <el-icon class="policy-help-icon"><InfoFilled /></el-icon>
                    </el-tooltip>
                  </span>
                </template>
                <el-switch v-model="bindPolicy.single_active_challenge_per_billing" />
              </el-form-item>
            </el-col>
          </el-row>
        </el-form>
      </template>
    </el-card>

    <el-card class="section-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-security"><el-icon><Clock /></el-icon></span>
            <span>绑定冷却名单</span>
          </div>
          <el-button size="small" :loading="bindCooldownsLoading" @click="reloadBindCooldowns">刷新名单</el-button>
        </div>
      </template>
      <el-alert
        title="展示当前处于绑定冷却期的平台账号，可查看失败次数和剩余时间，并支持立即解冻。"
        type="info"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-table :data="bindCooldownRows" stripe size="small" max-height="280" empty-text="暂无处于冷却中的账号">
        <el-table-column prop="billing_username" label="平台账号" min-width="170" />
        <el-table-column label="失败次数" width="100" align="center">
          <template #default="{ row }">
            <el-tag type="warning" size="small">{{ Number(row.failure_streak || 0) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="冷却到期" min-width="170">
          <template #default="{ row }">{{ fmtTime(row.cooldown_until || "") }}</template>
        </el-table-column>
        <el-table-column label="剩余时长" min-width="150">
          <template #default="{ row }">
            <el-text :type="bindCooldownActive(row) ? 'danger' : 'info'">{{ bindCooldownRemainingText(row) }}</el-text>
          </template>
        </el-table-column>
        <el-table-column label="活跃challenge" min-width="220">
          <template #default="{ row }">
            <span v-if="row.active_challenge_id">
              {{ row.active_challenge_node_id || "-" }}/{{ row.active_challenge_local_username || "-" }}
              （到期 {{ fmtTime(row.active_challenge_expires_at || "") }}）
            </span>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button
              size="small"
              type="warning"
              :disabled="!bindCooldownActive(row)"
              :loading="bindCooldownUnfreezeKey === row.billing_username"
              @click="unfreezeBindCooldown(row)"
            >
              立即解冻
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card risk-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-risk"><el-icon><WarningFilled /></el-icon></span>
            <span>异常换绑监测</span>
            <el-badge :value="mappingRisks.length" :hidden="mappingRisks.length <= 0" type="danger" class="pending-count-badge" />
          </div>
          <div class="head-actions">
            <el-input-number v-model="riskDays" :min="1" :max="365" :step="1" size="small" style="width: 130px" />
            <el-input-number v-model="riskMinSwitches" :min="1" :max="20" :step="1" size="small" style="width: 130px" />
            <el-button size="small" :loading="mappingRiskLoading" @click="reloadMappingRisks">刷新监测</el-button>
          </div>
        </div>
      </template>
      <el-alert
        title="检测规则：同一“节点编号+节点账号”在监测窗口内换绑次数较多，或涉及多个平台账号时标记为风险。"
        type="warning"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-table :data="mappingRisks" stripe size="small" max-height="260" empty-text="暂无异常换绑账号">
        <el-table-column label="风险" width="70">
          <template #default>
            <span class="risk-dot" />
          </template>
        </el-table-column>
        <el-table-column prop="node_id" label="节点编号" width="120" />
        <el-table-column prop="local_username" label="节点账号" width="150" />
        <el-table-column prop="current_billing_username" label="当前平台账号" width="170" />
        <el-table-column label="风险指标" width="240">
          <template #default="{ row }">
            <el-tag type="danger" size="small">换绑 {{ Number(row.switch_count || 0) }} 次</el-tag>
            <el-tag type="warning" size="small" style="margin-left: 6px">涉及 {{ Number(row.distinct_billing_count || 0) }} 人</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="换绑轨迹" min-width="320">
          <template #default="{ row }">
            <div v-if="(row.switch_history || []).length" class="switch-history-wrap">
              <div v-for="(item, idx) in row.switch_history || []" :key="`${row.node_id}:${row.local_username}:sw:${idx}`" class="switch-history-item">
                <span>{{ formatSwitchHistoryItem(item) }}</span>
              </div>
            </div>
            <span v-else class="mini">暂无明确换绑链路</span>
          </template>
        </el-table-column>
        <el-table-column label="涉及平台账号（可点击详情/拉黑）" min-width="380">
          <template #default="{ row }">
            <div class="risk-users">
              <div v-for="u in row.platform_usernames || []" :key="`${row.node_id}:${row.local_username}:${u}`" class="risk-user-item">
                <el-button link type="primary" @click="openProfile(u)">{{ u }}</el-button>
                <el-tag v-if="isPlatformBlocked(u)" type="danger" size="small">已拉黑</el-tag>
                <el-button
                  size="small"
                  :type="isPlatformBlocked(u) ? 'warning' : 'danger'"
                  plain
                  @click="toggleRiskUserBlock(u, row)"
                >
                  {{ isPlatformBlocked(u) ? "解黑" : "拉黑" }}
                </el-button>
              </div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="最近变更时间" min-width="170">
          <template #default="{ row }">{{ fmtTime(row.last_changed_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="120" fixed="right">
          <template #default="{ row }">
            <el-button
              type="info"
              plain
              size="small"
              :loading="mappingRiskClearKey === mappingKey(row.node_id, row.local_username)"
              @click="clearMappingRisk(row)"
            >
              清除
            </el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card mapping-query-card">
      <template #header>
        <div class="section-title-wrap">
          <span class="section-icon tone-query"><el-icon><Search /></el-icon></span>
          <span>映射查询</span>
        </div>
      </template>
      <el-form inline class="query-form">
        <el-form-item label="平台账号查询">
          <el-autocomplete
            v-model="filterBilling"
            style="width: 260px"
            clearable
            placeholder="输入平台账号筛选"
            :fetch-suggestions="queryBillingOptions"
            @select="onFilterBillingSelect"
          />
        </el-form-item>
        <el-form-item label="节点编号查询">
          <el-autocomplete
            v-model="filterNodeID"
            style="width: 220px"
            clearable
            placeholder="输入节点编号筛选"
            :fetch-suggestions="queryNodeOptions"
            @select="onFilterNodeIDSelect"
          />
        </el-form-item>
        <el-form-item label="节点账号查询">
          <el-autocomplete
            v-model="filterLocalUsername"
            style="width: 240px"
            clearable
            placeholder="输入节点账号筛选"
            :fetch-suggestions="queryLocalUserOptions"
            @select="onFilterLocalUsernameSelect"
          />
        </el-form-item>
        <el-form-item><el-button @click="reload">查询</el-button></el-form-item>
        <el-form-item><el-button @click="resetFilter">重置</el-button></el-form-item>
      </el-form>
    </el-card>

    <el-card class="section-card mapping-list-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-list"><el-icon><List /></el-icon></span>
            <span>映射列表</span>
          </div>
          <el-tag type="info" effect="plain">共 {{ rows.length }} 条</el-tag>
        </div>
      </template>
      <el-table :data="rows" stripe>
        <el-table-column label="平台账号" width="190">
          <template #default="{ row }">
            <div class="map-user-cell">
              <span v-if="isRiskRow(row)" class="risk-dot mini" />
              <el-button link type="primary" @click="openProfile(row.billing_username)">{{ row.billing_username }}</el-button>
            </div>
          </template>
        </el-table-column>
        <el-table-column prop="node_id" label="节点编号" width="140" />
        <el-table-column prop="local_username" label="节点账号" width="190" />
        <el-table-column label="更新时间" min-width="180">
          <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" width="320">
          <template #default="{ row }">
            <el-button size="small" @click="openEditDialog(row)">编辑</el-button>
            <el-button size="small" type="warning" plain @click="submitUnbindRequest(row)">代提解绑</el-button>
            <el-button size="small" type="danger" @click="remove(row)">强制解绑</el-button>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card restricted-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-note"><el-icon><WarningFilled /></el-icon></span>
            <span>受限名单（CPU/内存/GPU/黑名单）</span>
          </div>
          <el-button size="small" :loading="restrictedLoading" @click="reloadRestrictedRows">刷新名单</el-button>
        </div>
      </template>
      <el-alert
        title="该名单聚合展示 CPU 手动限制、内存手动限制、GPU 可见限制、SSH 黑名单。可在此查看受限类型并单项解除。"
        type="warning"
        :closable="false"
        show-icon
        class="mb"
      />
      <el-table :data="restrictedRows" stripe size="small" max-height="340" empty-text="暂无受限记录">
        <el-table-column prop="node_id" label="节点编号" width="120" />
        <el-table-column prop="local_username" label="节点账号" width="160" />
        <el-table-column label="平台账号" min-width="160">
          <template #default="{ row }">
            <el-button v-if="row.billing_username" link type="primary" @click="openProfile(row.billing_username)">
              {{ row.billing_username }}
            </el-button>
            <span v-else>-</span>
          </template>
        </el-table-column>
        <el-table-column label="映射状态" width="120" align="center">
          <template #default="{ row }">
            <el-tag v-if="row.mapping_exists && row.platform_exists" type="success" size="small">已映射</el-tag>
            <el-tag v-else-if="row.mapping_exists" type="warning" size="small">映射异常</el-tag>
            <el-tag v-else type="info" size="small">未映射</el-tag>
          </template>
        </el-table-column>
        <el-table-column label="受限项" min-width="220">
          <template #default="{ row }">
            <el-space wrap>
              <el-tag v-if="hasRestrictedCPU(row)" type="warning" size="small">
                CPU {{ Number(row.cpu_quota_percent || 0).toFixed(1) }}%
              </el-tag>
              <el-tag v-if="hasRestrictedMemory(row)" type="danger" size="small">
                内存 {{ Number(row.memory_limit_gb || 0).toFixed(1) }} GB
              </el-tag>
              <el-tag v-if="hasRestrictedGPU(row)" type="success" size="small">
                GPU {{ formatGPUIndices(row.gpu_indices) }}
              </el-tag>
              <el-tag v-if="hasRestrictedBlacklist(row)" type="danger" effect="dark" size="small">
                SSH黑名单
              </el-tag>
            </el-space>
          </template>
        </el-table-column>
        <el-table-column label="限制详情" min-width="260">
          <template #default="{ row }">
            <div class="restricted-details">
              <div v-if="hasRestrictedCPU(row)">CPU：{{ row.cpu_reason || "-" }}</div>
              <div v-if="hasRestrictedMemory(row)">内存：{{ row.memory_reason || "-" }}</div>
              <div v-if="hasRestrictedGPU(row)">GPU：{{ row.gpu_reason || "-" }}</div>
              <div v-if="hasRestrictedBlacklist(row)">黑名单：{{ row.blacklist_reason || "-" }}</div>
            </div>
          </template>
        </el-table-column>
        <el-table-column label="最后更新时间" min-width="170">
          <template #default="{ row }">{{ fmtTime(row.updated_at) }}</template>
        </el-table-column>
        <el-table-column label="操作" min-width="360" fixed="right">
          <template #default="{ row }">
            <el-space>
              <el-button
                size="small"
                type="warning"
                :loading="restrictedActionKey === `${row.node_id}::${row.local_username}::edit`"
                @click="openRestrictedLimitDialog(row)"
              >
                限制
              </el-button>
              <el-button
                size="small"
                type="info"
                plain
                :disabled="!hasRestrictedCPU(row)"
                :loading="restrictedActionKey === `${row.node_id}::${row.local_username}::clear-cpu`"
                @click="clearRestrictedCPULimit(row)"
              >
                解除CPU
              </el-button>
              <el-button
                size="small"
                type="info"
                plain
                :disabled="!hasRestrictedMemory(row)"
                :loading="restrictedActionKey === `${row.node_id}::${row.local_username}::clear-memory`"
                @click="clearRestrictedMemoryLimit(row)"
              >
                解除内存
              </el-button>
              <el-button
                size="small"
                type="success"
                plain
                :disabled="!hasRestrictedGPU(row)"
                :loading="restrictedActionKey === `${row.node_id}::${row.local_username}::clear-gpu`"
                @click="clearRestrictedGPUVisibility(row)"
              >
                解除GPU
              </el-button>
              <el-button
                size="small"
                type="danger"
                plain
                :disabled="!hasRestrictedBlacklist(row)"
                :loading="restrictedActionKey === `${row.node_id}::${row.local_username}::clear-blacklist`"
                @click="clearRestrictedBlacklist(row)"
              >
                解黑
              </el-button>
            </el-space>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-card class="section-card unbind-records-card">
      <template #header>
        <div class="head">
          <div class="section-title-wrap">
            <span class="section-icon tone-list"><el-icon><Document /></el-icon></span>
            <span>解绑记录</span>
            <el-badge
              :value="pendingUnbindRequestCount"
              :hidden="pendingUnbindRequestCount <= 0"
              type="danger"
              class="pending-count-badge"
            />
          </div>
          <div class="head-actions">
            <el-button size="small" plain @click="openUnbindRejectHistoryDialog">驳回历史</el-button>
            <el-button size="small" :loading="unbindRecordsLoading" @click="reloadUnbindRecords">刷新记录</el-button>
          </div>
        </div>
      </template>
      <el-table :data="displayUnbindRecords" stripe size="small" max-height="280" empty-text="暂无解绑记录">
        <el-table-column prop="created_at" label="记录时间" min-width="170">
          <template #default="{ row }">{{ fmtTime(row.created_at) }}</template>
        </el-table-column>
        <el-table-column prop="source_type" label="来源" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="String(row.source_type) === 'admin_forced' ? 'danger' : 'warning'">
              {{ String(row.source_type) === "admin_forced" ? "管理员强制" : "用户申请" }}
            </el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="status" label="结果" width="110">
          <template #default="{ row }">
            <el-tag size="small" :type="unbindStatusTagType(row.status)">{{ unbindStatusText(row.status) }}</el-tag>
          </template>
        </el-table-column>
        <el-table-column prop="billing_username" label="平台账号" width="170" />
        <el-table-column prop="node_id" label="节点编号" width="130" />
        <el-table-column prop="local_username" label="节点账号" width="150" />
        <el-table-column prop="reason" label="理由" min-width="260" />
        <el-table-column label="处理信息" min-width="220">
          <template #default="{ row }">
            <div class="mini">发起人：{{ row.initiated_by || "-" }}</div>
            <div class="mini">审批人：{{ row.reviewed_by || "-" }}</div>
            <div class="mini">执行时间：{{ fmtTime(row.executed_at || "") }}</div>
          </template>
        </el-table-column>
        <el-table-column label="操作" width="220" fixed="right">
          <template #default="{ row }">
            <el-space v-if="canReviewUnbindRecord(row)">
              <el-button
                size="small"
                type="success"
                :loading="unbindRecordActionLoadingRequestId === Number(row.request_id || 0)"
                @click="approveUnbindRecord(row)"
              >
                通过
              </el-button>
              <el-button
                size="small"
                type="danger"
                :loading="unbindRecordActionLoadingRequestId === Number(row.request_id || 0)"
                @click="rejectUnbindRecord(row)"
              >
                拒绝
              </el-button>
            </el-space>
            <span v-else class="mini">-</span>
          </template>
        </el-table-column>
      </el-table>
    </el-card>

    <el-dialog v-model="unbindRejectHistoryVisible" title="解绑申请驳回历史" width="1050px" destroy-on-close>
      <div class="head-actions mb">
        <el-button size="small" :loading="unbindRejectHistoryLoading" @click="reloadUnbindRejectHistory">刷新</el-button>
      </div>
      <el-table :data="unbindRejectHistoryRows" stripe size="small" max-height="460" empty-text="暂无驳回记录">
        <el-table-column prop="record_id" label="记录ID" width="90" />
        <el-table-column prop="request_id" label="申请ID" width="90" />
        <el-table-column prop="billing_username" label="平台账号" width="170" />
        <el-table-column prop="node_id" label="节点编号" width="130" />
        <el-table-column prop="local_username" label="节点账号" width="150" />
        <el-table-column prop="reason" label="驳回理由" min-width="260" />
        <el-table-column label="审核信息" min-width="220">
          <template #default="{ row }">
            <div class="mini">审核人：{{ row.reviewed_by || "-" }}</div>
            <div class="mini">审核时间：{{ fmtTime(row.reviewed_at || "") }}</div>
            <div class="mini">申请时间：{{ fmtTime(row.created_at || "") }}</div>
          </template>
        </el-table-column>
      </el-table>
      <template #footer>
        <el-button @click="unbindRejectHistoryVisible = false">关闭</el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="editVisible"
      :title="editMode === 'create' ? '新增映射' : '编辑映射'"
      width="620px"
      destroy-on-close
    >
      <el-form label-width="100px">
        <el-form-item label="平台账号">
          <el-autocomplete
            v-model="formBilling"
            style="width: 100%"
            clearable
            placeholder="输入平台账号"
            :fetch-suggestions="queryBillingOptions"
            @select="onFormBillingSelect"
          />
        </el-form-item>
        <el-form-item label="节点编号">
          <el-autocomplete
            v-model="nodeId"
            style="width: 100%"
            clearable
            placeholder="输入节点编号，例如 60000"
            :fetch-suggestions="queryNodeOptions"
            @select="onNodeSelect"
          />
        </el-form-item>
        <el-form-item label="节点账号">
          <el-autocomplete
            v-model="localUsername"
            style="width: 100%"
            clearable
            placeholder="输入节点账号"
            :fetch-suggestions="queryLocalUserOptions"
            @select="onLocalUserSelect"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="editVisible = false">取消</el-button>
        <el-button type="primary" :loading="saving" @click="save">
          {{ editMode === "create" ? "新增" : "保存修改" }}
        </el-button>
      </template>
    </el-dialog>

    <el-dialog
      v-model="restrictedLimitDialogVisible"
      title="用户限制设置"
      width="620px"
      destroy-on-close
    >
      <el-form label-width="110px">
        <el-form-item label="节点">
          <el-text>{{ restrictedLimitForm.node_id || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="节点账号">
          <el-text>{{ restrictedLimitForm.local_username || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="平台账号">
          <el-text>{{ restrictedLimitForm.billing_username || "-" }}</el-text>
        </el-form-item>
        <el-form-item label="限制 CPU">
          <el-switch v-model="restrictedLimitForm.cpu_enabled" inline-prompt active-text="是" inactive-text="否" />
          <el-input-number
            v-model="restrictedLimitForm.cpu_quota_percent"
            :disabled="!restrictedLimitForm.cpu_enabled"
            :min="1"
            :max="100"
            :step="1"
            :precision="1"
            style="margin-left: 10px; width: 140px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">单位：%</el-text>
        </el-form-item>
        <el-form-item label="限制内存">
          <el-switch v-model="restrictedLimitForm.memory_enabled" inline-prompt active-text="是" inactive-text="否" />
          <el-input-number
            v-model="restrictedLimitForm.memory_limit_gb"
            :disabled="!restrictedLimitForm.memory_enabled"
            :min="0.5"
            :max="4096"
            :step="0.5"
            :precision="1"
            style="margin-left: 10px; width: 140px"
          />
          <el-text type="info" size="small" style="margin-left: 8px">单位：GB</el-text>
        </el-form-item>
        <el-form-item label="限制 GPU 可见">
          <el-switch v-model="restrictedLimitForm.gpu_enabled" inline-prompt active-text="是" inactive-text="否" />
          <el-checkbox-group
            v-model="restrictedLimitForm.gpu_indices"
            :disabled="!restrictedLimitForm.gpu_enabled || restrictedGPUOptions.length === 0"
            style="margin-left: 10px"
          >
            <el-checkbox v-for="idx in restrictedGPUOptions" :key="`restricted-gpu-${idx}`" :label="idx">
              GPU {{ idx }}
            </el-checkbox>
          </el-checkbox-group>
          <el-text v-if="restrictedGPUOptions.length === 0" type="info" size="small" style="margin-left: 8px">
            当前节点未检测到可配置的 GPU 编号
          </el-text>
        </el-form-item>
        <el-form-item label="限制原因">
          <el-input
            v-model="restrictedLimitForm.reason"
            type="textarea"
            :rows="3"
            clearable
            maxlength="160"
            show-word-limit
            placeholder="可选：例如手动管控/占用过高/违规任务"
          />
        </el-form-item>
      </el-form>
      <template #footer>
        <el-button @click="restrictedLimitDialogVisible = false">取消</el-button>
        <el-button type="warning" :loading="restrictedLimitSaving" @click="saveRestrictedLimitForm">保存限制</el-button>
      </template>
    </el-dialog>

    <PlatformUserDetailDialog v-model="profileVisible" :username="selectedProfileUsername" />
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref } from "vue";
import { ElMessage, ElMessageBox } from "element-plus";
import {
  type AdminUserNodeBindCooldownRow,
  ApiClient,
  type AdminAccountProvisionLog,
  type AdminAccountProvisionResp,
  type AdminUserDetail,
  type NodeBindSecurityPolicy,
  type NodeUserGPUVisibility,
  type PlatformUserDetail,
  type SSHBlacklistEntry,
  type UserRequest,
  type UserNodeAccount,
  type UserNodeUnbindRecord,
  type UserNodeAccountMappingRisk,
} from "../../lib/api";
import { settingsState } from "../../lib/settingsStore";
import { authState } from "../../lib/authStore";
import { formatServerDateTime, toServerEpochMs } from "../../lib/time";
import { writeClipboardText } from "../../lib/clipboard";
import PlatformUserDetailDialog from "../../components/PlatformUserDetailDialog.vue";
import { Clock, Connection, Document, InfoFilled, Key, List, Search, UserFilled, WarningFilled } from "@element-plus/icons-vue";

const loading = ref(false);
const saving = ref(false);
const provisioning = ref(false);
const error = ref("");
const success = ref("");
const rows = ref<UserNodeAccount[]>([]);
const mappingRiskLoading = ref(false);
const mappingRiskClearKey = ref("");
const mappingRisks = ref<UserNodeAccountMappingRisk[]>([]);
const bindPolicyLoading = ref(false);
const bindPolicySaving = ref(false);
const bindCooldownsLoading = ref(false);
const bindCooldownUnfreezeKey = ref("");
const bindCooldownRows = ref<AdminUserNodeBindCooldownRow[]>([]);
const bindPolicy = reactive<NodeBindSecurityPolicy>({
  challenge_window_seconds: 300,
  trial_cpu_quota_percent: 20,
  trial_memory_limit_gb: 8,
  trial_gpu_blocked: true,
  single_active_challenge_per_billing: true,
  first_failure_cooldown_minutes: 30,
  repeat_failure_cooldown_minutes: 180,
  contention_freeze_minutes: 30,
});
type RestrictedRow = {
  node_id: string;
  local_username: string;
  billing_username: string;
  mapping_exists: boolean;
  platform_exists: boolean;
  cpu_quota_percent?: number;
  cpu_reason?: string;
  cpu_updated_at?: string;
  memory_limit_gb?: number;
  memory_reason?: string;
  memory_updated_at?: string;
  gpu_indices?: number[];
  gpu_reason?: string;
  gpu_updated_at?: string;
  blacklisted?: boolean;
  blacklist_reason?: string;
  blacklist_updated_at?: string;
  source_type?: string;
  source_platform_username?: string;
  updated_at: string;
};
const restrictedLoading = ref(false);
const restrictedRows = ref<RestrictedRow[]>([]);
const restrictedActionKey = ref("");
const restrictedLimitDialogVisible = ref(false);
const restrictedLimitSaving = ref(false);
const restrictedLimitForm = reactive({
  node_id: "",
  local_username: "",
  billing_username: "",
  cpu_enabled: false,
  cpu_quota_percent: 50,
  memory_enabled: false,
  memory_limit_gb: 8,
  gpu_enabled: false,
  gpu_indices: [] as number[],
  reason: "",
});
const riskDays = ref(30);
const riskMinSwitches = ref(2);
const filterBilling = ref("");
const filterNodeID = ref("");
const filterLocalUsername = ref("");
const formBilling = ref("");
const nodeId = ref("");
const localUsername = ref("");
const billingOptions = ref<string[]>([]);
const localUserOptions = ref<string[]>([]);
const nodeOptions = ref<string[]>([]);
const nodeIPByID = ref<Record<string, string>>({});
const nodeGPUCountByID = ref<Record<string, number>>({});
const platformUsers = ref<AdminUserDetail[]>([]);
const profileVisible = ref(false);
const selectedProfileUsername = ref("");
const editVisible = ref(false);
const editMode = ref<"create" | "edit">("create");
const old = ref<{ billing: string; node: string; local: string } | null>(null);

const provisionResultVisible = ref(false);
const provisionResult = ref<AdminAccountProvisionResp | null>(null);
const provisionLogsLoading = ref(false);
const provisionLogs = ref<AdminAccountProvisionLog[]>([]);
const openRequestsLoading = ref(false);
const openRequests = ref<UserRequest[]>([]);
const openRejectHistoryVisible = ref(false);
const openRejectHistoryLoading = ref(false);
const openRejectHistoryRows = ref<UserRequest[]>([]);
const unbindRecordsLoading = ref(false);
const unbindRecords = ref<UserNodeUnbindRecord[]>([]);
const unbindRejectHistoryVisible = ref(false);
const unbindRejectHistoryLoading = ref(false);
const unbindRejectHistoryRows = ref<UserNodeUnbindRecord[]>([]);
const openRequestStatus = ref<"pending" | "approved" | "rejected" | "">("pending");
const openRequestActionLoadingId = ref(0);
const unbindRecordActionLoadingRequestId = ref(0);
const pendingOpenRequests = ref<UserRequest[]>([]);
const pendingUnbindRequests = ref<UserRequest[]>([]);
const blockedIdentities = ref<Set<string>>(new Set());
const provisionUserLoading = ref(false);
const provisionUserError = ref("");
const provisionActionError = ref("");
const provisionActionSuccess = ref("");
const provisionUserDetail = ref<PlatformUserDetail | null>(null);
const provisionUserLastFetched = ref("");
let provisionUserFetchSeq = 0;
const DEFAULT_PROVISION_SSH_HOST = "controller.example.org";
const PROVISION_SSH_HOST_STORAGE_KEY = "gpuops.provision.last_ssh_host";

function loadProvisionSSHHostDefault(): string {
  try {
    const v = String(localStorage.getItem(PROVISION_SSH_HOST_STORAGE_KEY) || "").trim();
    return v || DEFAULT_PROVISION_SSH_HOST;
  } catch {
    return DEFAULT_PROVISION_SSH_HOST;
  }
}

function rememberProvisionSSHHost(host: string) {
  const v = String(host || "").trim();
  if (!v) return;
  try {
    localStorage.setItem(PROVISION_SSH_HOST_STORAGE_KEY, v);
  } catch {
    // ignore storage errors
  }
}

function rememberProvisionSSHHostFromForm() {
  rememberProvisionSSHHost(provisionForm.ssh_host);
}

const provisionForm = reactive({
  billing_username: "",
  node_id: "",
  local_username: "",
  ssh_host: loadProvisionSSHHostDefault(),
  ssh_port: 0,
});

const pendingOpenRequestCount = computed(() => pendingOpenRequests.value.length);
const pendingUnbindRequestCount = computed(() => pendingUnbindRequests.value.length);
const displayUnbindRecords = computed<UserNodeUnbindRecord[]>(() => {
  const base = [...(unbindRecords.value || [])];
  const existingRequestIDs = new Set<number>();
  for (const row of base) {
    const rid = Number(row.request_id || 0);
    if (Number.isFinite(rid) && rid > 0) existingRequestIDs.add(rid);
  }
  let syntheticID = -1;
  for (const req of pendingUnbindRequests.value || []) {
    if (String(req.request_type || "").trim() !== "unbind") continue;
    const requestID = Number(req.request_id || 0);
    if (Number.isFinite(requestID) && requestID > 0 && existingRequestIDs.has(requestID)) continue;
    base.push({
      record_id: syntheticID--,
      source_type: "user_request",
      request_id: Number.isFinite(requestID) && requestID > 0 ? requestID : undefined,
      billing_username: String(req.billing_username || "").trim(),
      node_id: String(req.node_id || "").trim(),
      local_username: String(req.local_username || "").trim(),
      status: "pending",
      reason: String(req.message || "").trim(),
      initiated_by: String(req.billing_username || "").trim() || "-",
      created_at: String(req.created_at || "").trim(),
      updated_at: String(req.updated_at || req.created_at || "").trim(),
    });
  }
  return base.sort((a, b) => {
    const ta = toServerEpochMs(String(a.created_at || ""));
    const tb = toServerEpochMs(String(b.created_at || ""));
    if (Number.isFinite(tb) && Number.isFinite(ta) && tb !== ta) return tb - ta;
    return Number(b.record_id || 0) - Number(a.record_id || 0);
  });
});

const pendingOpenRequestUsersText = computed(() => {
  const users = uniqSorted(pendingOpenRequests.value.map((x) => String(x.billing_username || "").trim()).filter(Boolean));
  if (!users.length) return "";
  const names = users.slice(0, 4).join("、");
  return users.length > 4 ? `${names} 等 ${users.length} 人` : names;
});

const pendingAlertTitle = computed(() => {
  if (pendingOpenRequestCount.value <= 0) return "";
  const base = `当前有 ${pendingOpenRequestCount.value} 条待处理的节点账号开通申请`;
  const users = pendingOpenRequestUsersText.value;
  return users ? `${base}（申请人：${users}）` : base;
});

const blacklistedPlatformUserSet = computed(() => {
  const set = new Set<string>();
  for (const x of blockedIdentities.value) {
    const v = String(x || "").trim();
    if (v) set.add(v);
  }
  return set;
});

function uniqSorted(items: string[]): string[] {
  const s = new Set<string>();
  for (const item of items) {
    const v = String(item || "").trim();
    if (v) s.add(v);
  }
  return Array.from(s).sort((a, b) => a.localeCompare(b));
}

function mappingKey(nodeID: string, localUser: string): string {
  return `${String(nodeID || "").trim()}::${String(localUser || "").trim()}`;
}

const riskKeySet = computed(() => {
  const set = new Set<string>();
  for (const r of mappingRisks.value || []) {
    const key = mappingKey(r.node_id, r.local_username);
    if (key) set.add(key);
  }
  return set;
});

function isRiskRow(row: UserNodeAccount): boolean {
  return riskKeySet.value.has(mappingKey(row.node_id, row.local_username));
}

function isPlatformBlocked(username: string): boolean {
  const u = String(username || "").trim();
  if (!u) return false;
  return blacklistedPlatformUserSet.value.has(u);
}

function formatSwitchHistoryItem(item: string): string {
  const s = String(item || "").trim();
  if (!s) return "";
  return s.replace(/\s*->\s*/g, " \u2192 ");
}

function fmtTime(v: string): string {
  return formatServerDateTime(v);
}

function normalizeGPUIndexList(v: number[]): number[] {
  const out = new Set<number>();
  for (const item of v || []) {
    const n = Number(item);
    if (!Number.isInteger(n) || n < 0) continue;
    out.add(n);
  }
  return Array.from(out).sort((a, b) => a - b);
}

function formatGPUIndices(indices?: number[]): string {
  const arr = normalizeGPUIndexList((indices || []).map((x) => Number(x)));
  if (!arr.length) return "-";
  return arr.join(", ");
}

const restrictedGPUOptions = computed(() => {
  const nodeId = String(restrictedLimitForm.node_id || "").trim();
  const count = Math.max(0, Number(nodeGPUCountByID.value[nodeId] || 0));
  return Array.from({ length: count }, (_, idx) => idx);
});

function bindCooldownRemainingText(row: AdminUserNodeBindCooldownRow): string {
  const sec = Number(row.remaining_cooldown_seconds || 0);
  if (!Number.isFinite(sec) || sec <= 0) return "-";
  const s = Math.floor(sec);
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const r = s % 60;
  if (h > 0) return `${h}小时${m}分${r}秒`;
  if (m > 0) return `${m}分${r}秒`;
  return `${r}秒`;
}

function bindCooldownActive(row: AdminUserNodeBindCooldownRow): boolean {
  return Number(row.remaining_cooldown_seconds || 0) > 0;
}

function collectBlockedPlatformUser(set: Set<string>, value: string) {
  const v = String(value || "").trim();
  if (!v) return;
  set.add(v);
}

function buildBlockedIdentitySet(entries: SSHBlacklistEntry[]): Set<string> {
  const set = new Set<string>();
  for (const e of entries || []) {
    const nodeID = String(e.node_id || "").trim();
    const sourceType = String(e.source_type || "").trim();
    if (nodeID !== "*" || sourceType !== "platform") {
      continue;
    }
    const sourcePlatform = String(e.source_platform_username || "").trim();
    const billing = String(e.billing_username || "").trim();
    if (sourcePlatform) {
      collectBlockedPlatformUser(set, sourcePlatform);
      continue;
    }
    if (billing) {
      collectBlockedPlatformUser(set, billing);
    }
  }
  return set;
}

function queryOptions(base: string[], queryString: string, cb: (items: Array<{ value: string }>) => void) {
  const q = String(queryString || "").trim().toLowerCase();
  cb(
    base
      .filter((x) => (q ? x.toLowerCase().includes(q) : true))
      .slice(0, 40)
      .map((x) => ({ value: x })),
  );
}

function queryBillingOptions(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  queryOptions(billingOptions.value, queryString, cb);
}

function queryNodeOptions(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  queryOptions(nodeOptions.value, queryString, cb);
}

function queryLocalUserOptions(queryString: string, cb: (items: Array<{ value: string }>) => void) {
  queryOptions(localUserOptions.value, queryString, cb);
}

function parseNodePort(node: string): number {
  const n = Number(String(node || "").trim());
  if (!Number.isFinite(n) || n <= 0 || n > 65535) return 0;
  return Math.floor(n);
}

function applyProvisionNodeDefaults(node: string) {
  const id = String(node || "").trim();
  if (!id) return;
  const ip = String(nodeIPByID.value[id] || "").trim();
  if (ip && !String(provisionForm.ssh_host || "").trim()) {
    provisionForm.ssh_host = ip;
  }
  if (!provisionForm.ssh_port || provisionForm.ssh_port <= 0) {
    const p = parseNodePort(id);
    provisionForm.ssh_port = p > 0 ? p : 22;
  }
}

function onFilterBillingSelect(item: { value?: string }) {
  filterBilling.value = String(item?.value || "").trim();
}

function onFilterNodeIDSelect(item: { value?: string }) {
  filterNodeID.value = String(item?.value || "").trim();
}

function onFilterLocalUsernameSelect(item: { value?: string }) {
  filterLocalUsername.value = String(item?.value || "").trim();
}

function onFormBillingSelect(item: { value?: string }) {
  formBilling.value = String(item?.value || "").trim();
}

function onNodeSelect(item: { value?: string }) {
  nodeId.value = String(item?.value || "").trim();
}

function onLocalUserSelect(item: { value?: string }) {
  localUsername.value = String(item?.value || "").trim();
}

function onProvisionBillingSelect(item: { value?: string }) {
  provisionForm.billing_username = String(item?.value || "").trim();
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  syncProvisionUserPreviewFromLocal();
  void fetchProvisionUserDetail(true);
}

function onProvisionBillingChange() {
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  syncProvisionUserPreviewFromLocal();
}

function onProvisionBillingBlur() {
  void fetchProvisionUserDetail(false);
}

function onProvisionNodeSelect(item: { value?: string }) {
  provisionForm.node_id = String(item?.value || "").trim();
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  applyProvisionNodeDefaults(provisionForm.node_id);
}

function currentMappingFilters() {
  return {
    billing_username: String(filterBilling.value || "").trim(),
    node_id: String(filterNodeID.value || "").trim(),
    local_username: String(filterLocalUsername.value || "").trim(),
  };
}

function openProfile(username: string) {
  selectedProfileUsername.value = String(username || "").trim();
  if (!selectedProfileUsername.value) return;
  profileVisible.value = true;
}

function fmt2(v: number): string {
  return Number(v || 0).toFixed(2);
}

function fmtGrad(year: number, month: number): string {
  if (!year || !month) return "-";
  return `${year}-${String(month).padStart(2, "0")}`;
}

function roleText(role: string): string {
  const r = String(role || "").trim();
  if (r === "admin") return "管理员";
  if (r === "power_user") return "高级用户";
  return "普通用户";
}

function statusText(status: string): string {
  const s = String(status || "").trim();
  if (s === "blocked") return "已封禁";
  if (s === "limited") return "受限";
  if (s === "warning") return "告警";
  return "正常";
}

function statusTagType(status: string): "success" | "warning" | "danger" | "info" {
  const s = String(status || "").trim();
  if (s === "blocked") return "danger";
  if (s === "limited" || s === "warning") return "warning";
  if (s === "normal") return "success";
  return "info";
}

function requestStatusText(status: string): string {
  const s = String(status || "").trim();
  if (s === "approved") return "已处理";
  if (s === "rejected") return "已拒绝";
  if (s === "pending") return "待处理";
  return s || "-";
}

function requestStatusTagType(status: string): "success" | "warning" | "danger" | "info" {
  const s = String(status || "").trim();
  if (s === "approved") return "success";
  if (s === "rejected") return "danger";
  if (s === "pending") return "warning";
  return "info";
}

function requestTypeText(requestType: string): string {
  const t = String(requestType || "").trim();
  if (t === "open") return "开通";
  if (t === "bind") return "绑定";
  if (t === "unbind") return "解绑";
  return t || "-";
}

function requestTypeTagType(requestType: string): "success" | "warning" | "danger" | "info" {
  const t = String(requestType || "").trim();
  if (t === "open") return "success";
  if (t === "bind") return "warning";
  if (t === "unbind") return "danger";
  return "info";
}

function unbindStatusText(status: string): string {
  const s = String(status || "").trim();
  if (s === "forced") return "已强制解绑";
  if (s === "approved") return "已审批解绑";
  if (s === "rejected") return "已拒绝";
  if (s === "pending") return "待审批";
  return s || "-";
}

function unbindStatusTagType(status: string): "success" | "warning" | "danger" | "info" {
  const s = String(status || "").trim();
  if (s === "forced") return "danger";
  if (s === "approved") return "success";
  if (s === "rejected") return "info";
  if (s === "pending") return "warning";
  return "info";
}

function findApplicant(username: string): AdminUserDetail | null {
  const u = String(username || "").trim();
  if (!u) return null;
  return (platformUsers.value || []).find((x) => String(x.username || "").trim() === u) || null;
}

function applicantSummary(username: string): string {
  const user = findApplicant(username);
  if (!user) return "未命中本地缓存，请点击平台账号查看完整信息";
  const realName = String(user.real_name || "").trim() || "-";
  const studentID = String(user.student_id || "").trim() || "-";
  const role = roleText(user.role);
  return `${realName}｜学号 ${studentID}｜级别 ${role}`;
}

function applicantContact(username: string): string {
  const user = findApplicant(username);
  if (!user) return "联系方式：-";
  const email = String(user.email || "").trim() || "-";
  const phone = String(user.phone || "").trim() || "-";
  return `邮箱 ${email}｜电话 ${phone}`;
}

function applicantAdvisor(username: string): string {
  const user = findApplicant(username);
  if (!user) return "导师/毕业时间：-";
  const advisor = String(user.advisor || "").trim() || "-";
  const grad = fmtGrad(user.expected_graduation_year, user.expected_graduation_month);
  return `导师 ${advisor}｜预计毕业 ${grad}`;
}

function toPlatformUserDetailFromAdminRow(row: AdminUserDetail): PlatformUserDetail {
  return {
    username: row.username,
    email: row.email,
    real_name: row.real_name,
    student_id: row.student_id,
    advisor: row.advisor,
    expected_graduation_year: row.expected_graduation_year,
    expected_graduation_month: row.expected_graduation_month,
    phone: row.phone,
    role: row.role,
    balance: Number(row.balance || 0),
    general_balance: Number(row.balance || 0),
    exclusive_balance: Number(row.exclusive_balance || 0),
    total_balance: Number(row.total_balance || (Number(row.balance || 0) + Number(row.exclusive_balance || 0))),
    status: row.status,
    node_accounts: row.node_accounts || [],
  };
}

function syncProvisionUserPreviewFromLocal() {
  const username = String(provisionForm.billing_username || "").trim();
  provisionUserError.value = "";
  if (!username) {
    provisionUserDetail.value = null;
    provisionUserLastFetched.value = "";
    return;
  }
  const row = (platformUsers.value || []).find((u) => String(u.username || "").trim() === username);
  if (row) {
    provisionUserDetail.value = toPlatformUserDetailFromAdminRow(row);
    return;
  }
  provisionUserDetail.value = null;
}

async function fetchProvisionUserDetail(force: boolean) {
  const username = String(provisionForm.billing_username || "").trim();
  if (!username) {
    provisionUserError.value = "";
    provisionUserDetail.value = null;
    provisionUserLastFetched.value = "";
    return;
  }
  if (!force && provisionUserLastFetched.value === username && provisionUserDetail.value?.username === username) {
    return;
  }
  const seq = ++provisionUserFetchSeq;
  provisionUserLoading.value = true;
  provisionUserError.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminPlatformUserDetail(username);
    if (seq !== provisionUserFetchSeq) return;
    provisionUserDetail.value = r.user;
    provisionUserLastFetched.value = username;
  } catch (e: any) {
    if (seq !== provisionUserFetchSeq) return;
    provisionUserError.value = e?.message ?? String(e);
  } finally {
    if (seq === provisionUserFetchSeq) {
      provisionUserLoading.value = false;
    }
  }
}

function resetFilter() {
  filterBilling.value = "";
  filterNodeID.value = "";
  filterLocalUsername.value = "";
  reload();
}

function clearEditForm() {
  formBilling.value = "";
  nodeId.value = "";
  localUsername.value = "";
  old.value = null;
}

function openCreateDialog() {
  clearEditForm();
  editMode.value = "create";
  editVisible.value = true;
}

function openEditDialog(row: UserNodeAccount) {
  editMode.value = "edit";
  old.value = { billing: row.billing_username, node: row.node_id, local: row.local_username };
  formBilling.value = row.billing_username;
  nodeId.value = row.node_id;
  localUsername.value = row.local_username;
  editVisible.value = true;
}

function mergeBillingOptions(accounts: UserNodeAccount[], users: AdminUserDetail[]) {
  const values: string[] = [];
  for (const a of accounts) values.push(a.billing_username || "");
  for (const u of users) values.push(u.username || "");
  billingOptions.value = uniqSorted(values);
}

function refreshLocalOptions(accounts: UserNodeAccount[]) {
  localUserOptions.value = uniqSorted(accounts.map((x) => x.local_username || ""));
}

async function loadNodeOptions() {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const r = await client.adminNodes(3000);
  const ids: string[] = [];
  const ipMap: Record<string, string> = {};
  const gpuCountMap: Record<string, number> = {};
  for (const n of r.nodes ?? []) {
    const id = String(n.node_id || "").trim();
    if (!id) continue;
    ids.push(id);
    ipMap[id] = String(n.node_ip || "").trim();
    gpuCountMap[id] = Math.max(0, Number(n.gpu_count || 0));
  }
  nodeOptions.value = uniqSorted(ids);
  nodeIPByID.value = ipMap;
  nodeGPUCountByID.value = gpuCountMap;
}

async function loadPlatformUsers() {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const r = await client.adminUsersDetails(3000);
  platformUsers.value = r.users ?? [];
  syncProvisionUserPreviewFromLocal();
}

async function reload() {
  loading.value = true;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const filters = currentMappingFilters();
    const r = await client.adminAccounts({
      billing_username: filters.billing_username || undefined,
      node_id: filters.node_id || undefined,
      local_username: filters.local_username || undefined,
    });
    rows.value = r.accounts ?? [];
    refreshLocalOptions(rows.value);
    mergeBillingOptions(rows.value, platformUsers.value);
    await Promise.all([
      reloadProvisionLogs(),
      reloadOpenRequests(),
      reloadUnbindRecords(),
      reloadBindCooldowns(),
      reloadMappingRisks(),
      reloadBlockedIdentities(),
      reloadRestrictedRows(),
    ]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    loading.value = false;
  }
}

async function reloadBindPolicy() {
  bindPolicyLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminGetBindPolicy();
    bindPolicy.challenge_window_seconds = Number(r.challenge_window_seconds ?? 300);
    bindPolicy.trial_cpu_quota_percent = Number(r.trial_cpu_quota_percent ?? 20);
    bindPolicy.trial_memory_limit_gb = Number(r.trial_memory_limit_gb ?? 8);
    bindPolicy.trial_gpu_blocked = !!r.trial_gpu_blocked;
    bindPolicy.single_active_challenge_per_billing = r.single_active_challenge_per_billing !== false;
    bindPolicy.first_failure_cooldown_minutes = Number(r.first_failure_cooldown_minutes ?? 30);
    bindPolicy.repeat_failure_cooldown_minutes = Number(r.repeat_failure_cooldown_minutes ?? 180);
    bindPolicy.contention_freeze_minutes = Number(r.contention_freeze_minutes ?? 30);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    bindPolicyLoading.value = false;
  }
}

async function saveBindPolicy() {
  bindPolicySaving.value = true;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminSetBindPolicy({ ...bindPolicy });
    Object.assign(bindPolicy, r.policy || bindPolicy);
    success.value = "绑定挑战策略已保存";
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    bindPolicySaving.value = false;
  }
}

async function reloadBindCooldowns() {
  bindCooldownsLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminBindCooldowns({ active_only: false, limit: 500 });
    const nowMs = Date.now();
    bindCooldownRows.value = (r.rows ?? [])
      .filter((row) => {
        const remain = Number(row.remaining_cooldown_seconds || 0);
        if (Number.isFinite(remain) && remain > 0) return true;
        const untilMs = toServerEpochMs(String(row.cooldown_until || ""));
        return Number.isFinite(untilMs) && untilMs > nowMs;
      })
      .sort((a, b) => {
        const ta = toServerEpochMs(String(a.cooldown_until || ""));
        const tb = toServerEpochMs(String(b.cooldown_until || ""));
        if (Number.isFinite(tb) && Number.isFinite(ta) && tb !== ta) return tb - ta;
        return Number(b.failure_streak || 0) - Number(a.failure_streak || 0);
      });
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    bindCooldownsLoading.value = false;
  }
}

async function unfreezeBindCooldown(row: AdminUserNodeBindCooldownRow) {
  const billing = String(row.billing_username || "").trim();
  if (!billing) return;
  try {
    await ElMessageBox.confirm(
      `确认立即解除平台账号 ${billing} 的绑定冷却吗？`,
      "立即解冻",
      { type: "warning", confirmButtonText: "确认解冻", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  bindCooldownUnfreezeKey.value = billing;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminUnfreezeBindCooldown({ billing_username: billing });
    success.value = `平台账号 ${billing} 已立即解冻`;
    await reloadBindCooldowns();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    bindCooldownUnfreezeKey.value = "";
  }
}

async function reloadMappingRisks() {
  mappingRiskLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminAccountMappingRisks({
      days: Number(riskDays.value || 30),
      min_switches: Number(riskMinSwitches.value || 2),
      limit: 500,
    });
    mappingRisks.value = r.risky_accounts ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    mappingRiskLoading.value = false;
  }
}

async function clearMappingRisk(row: UserNodeAccountMappingRisk) {
  const nodeId = String(row.node_id || "").trim();
  const local = String(row.local_username || "").trim();
  if (!nodeId || !local) return;
  try {
    await ElMessageBox.confirm(
      `确认清除异常换绑条目吗？\n节点：${nodeId}\n节点账号：${local}\n\n说明：只清除当前监测消息，后续若再次发生换绑会重新出现。`,
      "清除确认",
      { type: "warning", confirmButtonText: "确认清除", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  const key = mappingKey(nodeId, local);
  mappingRiskClearKey.value = key;
  error.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminClearAccountMappingRisk({ node_id: nodeId, local_username: local });
    success.value = `已清除异常换绑条目：${nodeId}/${local}`;
    mappingRisks.value = (mappingRisks.value || []).filter((x) => mappingKey(x.node_id, x.local_username) !== key);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    mappingRiskClearKey.value = "";
  }
}

function restrictedKey(nodeId: string, localUsername: string): string {
  return `${String(nodeId || "").trim()}::${String(localUsername || "").trim()}`;
}

function latestTimestamp(values: string[]): string {
  let chosen = "";
  let chosenMS = 0;
  for (const v of values) {
    const text = String(v || "").trim();
    if (!text) continue;
    const ms = toServerEpochMs(text);
    if (!Number.isFinite(ms)) continue;
    if (!chosen || ms >= chosenMS) {
      chosen = text;
      chosenMS = ms;
    }
  }
  if (chosen) return chosen;
  for (const v of values) {
    const text = String(v || "").trim();
    if (text) return text;
  }
  return "";
}

function hasRestrictedCPU(row: RestrictedRow): boolean {
  return Number(row.cpu_quota_percent || 0) > 0;
}

function hasRestrictedMemory(row: RestrictedRow): boolean {
  return Number(row.memory_limit_gb || 0) > 0;
}

function hasRestrictedGPU(row: RestrictedRow): boolean {
  return normalizeGPUIndexList((row.gpu_indices || []).map((x) => Number(x))).length > 0;
}

function hasRestrictedBlacklist(row: RestrictedRow): boolean {
  return !!row.blacklisted;
}

async function deleteCPULimitSafe(nodeId: string, localUsername: string) {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await client.adminDeleteNodeUserCPULimit(nodeId, localUsername);
  } catch (e: any) {
    const status = Number(e?.status || 0);
    if (status !== 404) throw e;
  }
}

async function deleteMemoryLimitSafe(nodeId: string, localUsername: string) {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await client.adminDeleteNodeUserMemoryLimit(nodeId, localUsername);
  } catch (e: any) {
    const status = Number(e?.status || 0);
    if (status !== 404) throw e;
  }
}

async function deleteGPUVisibilitySafe(nodeId: string, localUsername: string) {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await client.adminDeleteNodeUserGPUVisibility(nodeId, localUsername);
  } catch (e: any) {
    const status = Number(e?.status || 0);
    if (status !== 404) throw e;
  }
}

async function deleteBlacklistSafe(nodeId: string, localUsername: string) {
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  try {
    await client.adminDeleteBlacklist(nodeId, localUsername);
  } catch (e: any) {
    const status = Number(e?.status || 0);
    if (status !== 404) throw e;
  }
}

async function reloadRestrictedRows() {
  restrictedLoading.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const filters = currentMappingFilters();
    const billingFilter = filters.billing_username || undefined;
    const [cpuResp, memoryResp, gpuResp, blacklistResp] = await Promise.all([
      client.adminCPULimits({ billingUsername: billingFilter, limit: 3000 }),
      client.adminMemoryLimits({ billingUsername: billingFilter, limit: 3000 }),
      client.adminGPUVisibility({ billingUsername: billingFilter, limit: 3000 }),
      client.adminBlacklist(""),
    ]);

    const map = new Map<string, RestrictedRow>();
    const mappingByKey = new Map<string, UserNodeAccount>();
    for (const acc of rows.value || []) {
      mappingByKey.set(restrictedKey(acc.node_id, acc.local_username), acc);
    }
    const platformSet = new Set<string>((platformUsers.value || []).map((u) => String(u.username || "").trim()).filter(Boolean));
    const ensureRow = (nodeId: string, localUsername: string): RestrictedRow => {
      const k = restrictedKey(nodeId, localUsername);
      const hit = map.get(k);
      if (hit) return hit;
      const meta = mappingByKey.get(k);
      const out: RestrictedRow = {
        node_id: String(nodeId || "").trim(),
        local_username: String(localUsername || "").trim(),
        billing_username: String(meta?.billing_username || "").trim(),
        mapping_exists: !!meta,
        platform_exists: !!(meta && platformSet.has(String(meta.billing_username || "").trim())),
        updated_at: "",
      };
      map.set(k, out);
      return out;
    };

    for (const r of cpuResp.rows || []) {
      const row = ensureRow(r.node_id, r.local_username);
      row.cpu_quota_percent = Number(r.cpu_quota_percent || 0);
      row.cpu_reason = String(r.reason || "").trim();
      row.cpu_updated_at = String(r.updated_at || "").trim();
      if (!row.billing_username) row.billing_username = String(r.billing_username || "").trim();
      if (r.mapping_exists) row.mapping_exists = true;
      if (r.platform_exists) row.platform_exists = true;
    }
    for (const r of memoryResp.rows || []) {
      const row = ensureRow(r.node_id, r.local_username);
      row.memory_limit_gb = Number(r.memory_limit_gb || 0);
      row.memory_reason = String(r.reason || "").trim();
      row.memory_updated_at = String(r.updated_at || "").trim();
      if (!row.billing_username) row.billing_username = String(r.billing_username || "").trim();
      if (r.mapping_exists) row.mapping_exists = true;
      if (r.platform_exists) row.platform_exists = true;
    }
    for (const r of gpuResp.rows || []) {
      const gpuRow = r as NodeUserGPUVisibility;
      const row = ensureRow(gpuRow.node_id, gpuRow.local_username);
      row.gpu_indices = normalizeGPUIndexList((gpuRow.gpu_indices || []).map((x) => Number(x)));
      row.gpu_reason = String(gpuRow.reason || "").trim();
      row.gpu_updated_at = String(gpuRow.updated_at || "").trim();
      if (!row.billing_username) row.billing_username = String(gpuRow.billing_username || "").trim();
      if (gpuRow.mapping_exists) row.mapping_exists = true;
      if (gpuRow.platform_exists) row.platform_exists = true;
    }
    for (const r of blacklistResp.entries || []) {
      const nodeId = String(r.node_id || "").trim();
      const localUsername = String(r.local_username || "").trim();
      if (!nodeId || !localUsername) continue;
      const row = ensureRow(nodeId, localUsername);
      row.blacklisted = true;
      row.blacklist_reason = String(r.reason || "").trim();
      row.blacklist_updated_at = String(r.updated_at || "").trim();
      row.source_type = String(r.source_type || "").trim();
      row.source_platform_username = String(r.source_platform_username || "").trim();
      if (!row.billing_username) {
        row.billing_username = String(r.billing_username || "").trim() || String(r.source_platform_username || "").trim();
      }
    }

    const billingKw = filters.billing_username.toLowerCase();
    const nodeKw = filters.node_id.toLowerCase();
    const localKw = filters.local_username.toLowerCase();
    const out = Array.from(map.values())
      .map((row) => {
        if (!row.platform_exists && row.billing_username && platformSet.has(String(row.billing_username || "").trim())) {
          row.platform_exists = true;
        }
        return row;
      })
      .filter((row) => {
        if (!hasRestrictedCPU(row) && !hasRestrictedMemory(row) && !hasRestrictedGPU(row) && !hasRestrictedBlacklist(row)) return false;
        const node = String(row.node_id || "").toLowerCase();
        const local = String(row.local_username || "").toLowerCase();
        const billing = String(row.billing_username || "").toLowerCase();
        const sourcePlatform = String(row.source_platform_username || "").toLowerCase();
        if (billingKw && !billing.includes(billingKw) && !sourcePlatform.includes(billingKw)) return false;
        if (nodeKw && !node.includes(nodeKw)) return false;
        if (localKw && !local.includes(localKw)) return false;
        return true;
      })
      .map((row) => ({
        ...row,
        updated_at: latestTimestamp([
          String(row.blacklist_updated_at || ""),
          String(row.memory_updated_at || ""),
          String(row.cpu_updated_at || ""),
          String(row.gpu_updated_at || ""),
        ]),
      }))
      .sort((a, b) => {
        const ta = toServerEpochMs(String(a.updated_at || ""));
        const tb = toServerEpochMs(String(b.updated_at || ""));
        if (Number.isFinite(ta) && Number.isFinite(tb) && ta !== tb) return tb - ta;
        const an = String(a.node_id || "").localeCompare(String(b.node_id || ""));
        if (an !== 0) return an;
        return String(a.local_username || "").localeCompare(String(b.local_username || ""));
      });
    restrictedRows.value = out;
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    restrictedLoading.value = false;
  }
}

function openRestrictedLimitDialog(row: RestrictedRow) {
  restrictedLimitForm.node_id = String(row.node_id || "").trim();
  restrictedLimitForm.local_username = String(row.local_username || "").trim();
  restrictedLimitForm.billing_username = String(row.billing_username || "").trim();
  const cpu = Number(row.cpu_quota_percent || 0);
  const memory = Number(row.memory_limit_gb || 0);
  const gpuIndices = normalizeGPUIndexList((row.gpu_indices || []).map((x) => Number(x)));
  restrictedLimitForm.cpu_enabled = cpu > 0;
  restrictedLimitForm.cpu_quota_percent = Number((cpu > 0 ? cpu : 50).toFixed(1));
  restrictedLimitForm.memory_enabled = memory > 0;
  restrictedLimitForm.memory_limit_gb = Number((memory > 0 ? memory : 8).toFixed(1));
  restrictedLimitForm.gpu_enabled = gpuIndices.length > 0;
  restrictedLimitForm.gpu_indices = [...gpuIndices];
  restrictedLimitForm.reason = String(row.cpu_reason || row.memory_reason || row.gpu_reason || "").trim();
  restrictedLimitDialogVisible.value = true;
}

async function saveRestrictedLimitForm() {
  const nodeId = String(restrictedLimitForm.node_id || "").trim();
  const local = String(restrictedLimitForm.local_username || "").trim();
  if (!nodeId || !local) return;
  const cpuEnabled = !!restrictedLimitForm.cpu_enabled;
  const memoryEnabled = !!restrictedLimitForm.memory_enabled;
  const gpuEnabled = !!restrictedLimitForm.gpu_enabled;
  const cpu = Number(restrictedLimitForm.cpu_quota_percent || 0);
  const memory = Number(restrictedLimitForm.memory_limit_gb || 0);
  const gpuIndices = normalizeGPUIndexList((restrictedLimitForm.gpu_indices || []).map((x) => Number(x)));
  if (cpuEnabled && (!Number.isFinite(cpu) || cpu <= 0 || cpu > 100)) {
    ElMessage.error("CPU 限制必须在 1~100 之间");
    return;
  }
  if (memoryEnabled && (!Number.isFinite(memory) || memory <= 0 || memory > 4096)) {
    ElMessage.error("内存限制必须在 0~4096 GB 之间");
    return;
  }
  if (gpuEnabled && gpuIndices.length === 0) {
    ElMessage.error("请至少选择一个可见 GPU");
    return;
  }
  restrictedActionKey.value = `${nodeId}::${local}::edit`;
  restrictedLimitSaving.value = true;
  try {
    const reason = String(restrictedLimitForm.reason || "").trim();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (cpuEnabled) {
      await client.adminSetNodeUserCPULimit(nodeId, {
        local_username: local,
        cpu_quota_percent: Number(cpu.toFixed(1)),
        reason,
      });
    } else {
      await deleteCPULimitSafe(nodeId, local);
    }
    if (memoryEnabled) {
      await client.adminSetNodeUserMemoryLimit(nodeId, {
        local_username: local,
        memory_limit_gb: Number(memory.toFixed(1)),
        reason,
      });
    } else {
      await deleteMemoryLimitSafe(nodeId, local);
    }
    if (gpuEnabled) {
      await client.adminSetNodeUserGPUVisibility(nodeId, {
        local_username: local,
        gpu_indices: gpuIndices,
        reason,
      });
    } else {
      await deleteGPUVisibilitySafe(nodeId, local);
    }
    restrictedLimitDialogVisible.value = false;
    success.value = `已更新 ${nodeId}/${local} 的限制配置`;
    await reloadRestrictedRows();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    restrictedActionKey.value = "";
    restrictedLimitSaving.value = false;
  }
}

async function clearRestrictedCPULimit(row: RestrictedRow) {
  const nodeId = String(row.node_id || "").trim();
  const local = String(row.local_username || "").trim();
  if (!nodeId || !local) return;
  try {
    await ElMessageBox.confirm(`确认解除 ${nodeId}/${local} 的 CPU 限制吗？`, "二次确认", {
      type: "warning",
      confirmButtonText: "确认解除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  restrictedActionKey.value = `${nodeId}::${local}::clear-cpu`;
  try {
    await deleteCPULimitSafe(nodeId, local);
    success.value = `已解除 ${nodeId}/${local} 的 CPU 限制`;
    await reloadRestrictedRows();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    restrictedActionKey.value = "";
  }
}

async function clearRestrictedMemoryLimit(row: RestrictedRow) {
  const nodeId = String(row.node_id || "").trim();
  const local = String(row.local_username || "").trim();
  if (!nodeId || !local) return;
  try {
    await ElMessageBox.confirm(`确认解除 ${nodeId}/${local} 的内存限制吗？`, "二次确认", {
      type: "warning",
      confirmButtonText: "确认解除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  restrictedActionKey.value = `${nodeId}::${local}::clear-memory`;
  try {
    await deleteMemoryLimitSafe(nodeId, local);
    success.value = `已解除 ${nodeId}/${local} 的内存限制`;
    await reloadRestrictedRows();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    restrictedActionKey.value = "";
  }
}

async function clearRestrictedGPUVisibility(row: RestrictedRow) {
  const nodeId = String(row.node_id || "").trim();
  const local = String(row.local_username || "").trim();
  if (!nodeId || !local) return;
  try {
    await ElMessageBox.confirm(`确认解除 ${nodeId}/${local} 的 GPU 可见限制吗？`, "二次确认", {
      type: "warning",
      confirmButtonText: "确认解除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  restrictedActionKey.value = `${nodeId}::${local}::clear-gpu`;
  try {
    await deleteGPUVisibilitySafe(nodeId, local);
    success.value = `已解除 ${nodeId}/${local} 的 GPU 可见限制`;
    await reloadRestrictedRows();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    restrictedActionKey.value = "";
  }
}

async function clearRestrictedBlacklist(row: RestrictedRow) {
  const nodeId = String(row.node_id || "").trim();
  const local = String(row.local_username || "").trim();
  if (!nodeId || !local) return;
  try {
    await ElMessageBox.confirm(`确认解除 ${nodeId}/${local} 的 SSH 黑名单吗？`, "二次确认", {
      type: "warning",
      confirmButtonText: "确认解除",
      cancelButtonText: "取消",
    });
  } catch {
    return;
  }
  restrictedActionKey.value = `${nodeId}::${local}::clear-blacklist`;
  try {
    await deleteBlacklistSafe(nodeId, local);
    success.value = `已解除 ${nodeId}/${local} 的 SSH 黑名单`;
    await Promise.all([reloadBlockedIdentities(), reloadRestrictedRows()]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    restrictedActionKey.value = "";
  }
}

async function reloadBlockedIdentities() {
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminBlacklist("");
    blockedIdentities.value = buildBlockedIdentitySet(r.entries ?? []);
  } catch {
    blockedIdentities.value = new Set<string>();
  }
}

async function toggleRiskUserBlock(username: string, row: UserNodeAccountMappingRisk) {
  const u = String(username || "").trim();
  if (!u) return;
  const blocked = isPlatformBlocked(u);
  const title = blocked ? "解黑确认" : "拉黑确认";
  const body = blocked
    ? `确认解除平台账号 ${u} 的黑名单吗？\n节点：${row.node_id}\n节点账号：${row.local_username}\n\n解除后该账号可重新按映射规则登录。`
    : `确认拉黑平台账号 ${u} 吗？\n节点：${row.node_id}\n节点账号：${row.local_username}\n\n拉黑后会覆盖其所有节点映射，立即断开 SSH 并清理进程。`;
  try {
    await ElMessageBox.confirm(
      body,
      title,
      {
        type: "warning",
        confirmButtonText: blocked ? "确认解黑" : "确认拉黑",
        cancelButtonText: "取消",
      },
    );
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (blocked) {
      await client.adminUnblockUser(u);
      success.value = `已解除平台账号 ${u} 黑名单`;
    } else {
      await client.adminBlockUser(u, `换绑风险处置：节点 ${row.node_id} 账号 ${row.local_username}`);
      success.value = `已拉黑平台账号 ${u}，并已对其全部映射节点生效`;
    }
    await loadPlatformUsers();
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function reloadProvisionLogs() {
  provisionLogsLoading.value = true;
  try {
    const filters = currentMappingFilters();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminProvisionLogs({
      billing_username: filters.billing_username || undefined,
      node_id: filters.node_id || undefined,
      local_username: filters.local_username || undefined,
      limit: 200,
    });
    provisionLogs.value = r.logs ?? [];
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    provisionLogsLoading.value = false;
  }
}

async function reloadUnbindRecords() {
  unbindRecordsLoading.value = true;
  try {
    const filters = currentMappingFilters();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminUnbindRecords({
      billing_username: filters.billing_username || undefined,
      node_id: filters.node_id || undefined,
      local_username: filters.local_username || undefined,
      limit: 300,
    });
    unbindRecords.value = r.records ?? [];
  } catch (e: any) {
    if (e?.status === 404) {
      // 兼容旧控制器：未升级到解绑记录接口时不阻塞页面其余功能。
      unbindRecords.value = [];
      return;
    }
    error.value = e?.message ?? String(e);
  } finally {
    unbindRecordsLoading.value = false;
  }
}

async function reloadOpenRequests() {
  openRequestsLoading.value = true;
  try {
    const filters = currentMappingFilters();
    const billingKw = filters.billing_username.toLowerCase();
    const nodeKw = filters.node_id.toLowerCase();
    const localKw = filters.local_username.toLowerCase();
    const matchMappingFilters = (x: UserRequest): boolean => {
      const billing = String(x.billing_username || "").toLowerCase();
      const node = String(x.node_id || "").toLowerCase();
      const local = String(x.local_username || "").toLowerCase();
      if (billingKw && !billing.includes(billingKw)) return false;
      if (nodeKw && !node.includes(nodeKw)) return false;
      if (localKw && !local.includes(localKw)) return false;
      return true;
    };
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminRequests({ status: openRequestStatus.value || "", limit: 5000 });
    const allCurrent = r.requests ?? [];
    const current = allCurrent.filter((x) => {
      const requestType = String(x.request_type || "").trim();
      return requestType === "open" && matchMappingFilters(x);
    });
    openRequests.value = current;
    if (!openRequestStatus.value || openRequestStatus.value === "pending") {
      pendingOpenRequests.value = allCurrent.filter(
        (x) => {
          const requestType = String(x.request_type || "").trim();
          return requestType === "open" && String(x.status || "").trim() === "pending" && matchMappingFilters(x);
        },
      );
      pendingUnbindRequests.value = allCurrent.filter(
        (x) => String(x.request_type || "").trim() === "unbind" && String(x.status || "").trim() === "pending" && matchMappingFilters(x),
      );
      return;
    }
    const pendingResp = await client.adminRequests({ status: "pending", limit: 5000 });
    const pendingAll = pendingResp.requests ?? [];
    pendingOpenRequests.value = pendingAll.filter((x) => {
      const requestType = String(x.request_type || "").trim();
      return requestType === "open" && matchMappingFilters(x);
    });
    pendingUnbindRequests.value = pendingAll.filter(
      (x) => String(x.request_type || "").trim() === "unbind" && matchMappingFilters(x),
    );
  } catch (e: any) {
    pendingOpenRequests.value = [];
    pendingUnbindRequests.value = [];
    error.value = e?.message ?? String(e);
  } finally {
    openRequestsLoading.value = false;
  }
}

async function reloadOpenRejectHistory() {
  openRejectHistoryLoading.value = true;
  try {
    const filters = currentMappingFilters();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminRequests({ status: "rejected", limit: 5000 });
    const billingKw = filters.billing_username.toLowerCase();
    const nodeKw = filters.node_id.toLowerCase();
    const localKw = filters.local_username.toLowerCase();
    openRejectHistoryRows.value = (r.requests ?? [])
      .filter((x) => {
        const requestType = String(x.request_type || "").trim();
        if (requestType !== "open") return false;
        const billing = String(x.billing_username || "").toLowerCase();
        const node = String(x.node_id || "").toLowerCase();
        const local = String(x.local_username || "").toLowerCase();
        if (billingKw && !billing.includes(billingKw)) return false;
        if (nodeKw && !node.includes(nodeKw)) return false;
        if (localKw && !local.includes(localKw)) return false;
        return true;
      })
      .sort((a, b) => {
        const ta = toServerEpochMs(String(a.reviewed_at || a.updated_at || a.created_at || ""));
        const tb = toServerEpochMs(String(b.reviewed_at || b.updated_at || b.created_at || ""));
        if (Number.isFinite(tb) && Number.isFinite(ta) && tb !== ta) return tb - ta;
        return Number(b.request_id || 0) - Number(a.request_id || 0);
      });
  } catch (e: any) {
    error.value = e?.message ?? String(e);
    openRejectHistoryRows.value = [];
  } finally {
    openRejectHistoryLoading.value = false;
  }
}

async function openOpenRejectHistoryDialog() {
  openRejectHistoryVisible.value = true;
  await reloadOpenRejectHistory();
}

async function reloadUnbindRejectHistory() {
  unbindRejectHistoryLoading.value = true;
  try {
    const filters = currentMappingFilters();
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminUnbindRecords({
      billing_username: filters.billing_username || undefined,
      node_id: filters.node_id || undefined,
      local_username: filters.local_username || undefined,
      status: "rejected",
      limit: 1000,
    });
    unbindRejectHistoryRows.value = (r.records ?? []).sort((a, b) => {
      const ta = toServerEpochMs(String(a.reviewed_at || a.updated_at || a.created_at || ""));
      const tb = toServerEpochMs(String(b.reviewed_at || b.updated_at || b.created_at || ""));
      if (Number.isFinite(tb) && Number.isFinite(ta) && tb !== ta) return tb - ta;
      return Number(b.record_id || 0) - Number(a.record_id || 0);
    });
  } catch (e: any) {
    error.value = e?.message ?? String(e);
    unbindRejectHistoryRows.value = [];
  } finally {
    unbindRejectHistoryLoading.value = false;
  }
}

async function openUnbindRejectHistoryDialog() {
  unbindRejectHistoryVisible.value = true;
  await reloadUnbindRejectHistory();
}

function focusPendingRequests() {
  openRequestStatus.value = "pending";
  void reloadOpenRequests();
}

function applyOpenRequestToProvision(row: UserRequest) {
  if (String(row.request_type || "").trim() !== "open") {
    ElMessage.info("仅“开通申请”支持带入开通表单");
    return;
  }
  const billing = String(row.billing_username || "").trim();
  if (!billing) return;
  provisionForm.billing_username = billing;
  const node = String(row.node_id || "").trim();
  const local = String(row.local_username || "").trim();
  if (node && node !== "待分配") {
    provisionForm.node_id = node;
    applyProvisionNodeDefaults(node);
  }
  if (local && local !== "待分配") {
    provisionForm.local_username = local;
  }
  syncProvisionUserPreviewFromLocal();
  void fetchProvisionUserDetail(true);
  provisionActionError.value = "";
  provisionActionSuccess.value = `已将申请 ${row.request_id} 带入开通表单，请核对后执行开通`;
  ElMessage.success("已带入开通表单，请在当前区域核对后执行开通");
}

async function markOpenRequestProcessed(row: UserRequest) {
  const requestID = Number(row.request_id || 0);
  if (!requestID || String(row.status || "").trim() !== "pending") return;
  try {
    await ElMessageBox.confirm(
      `确认将申请 ${requestID} 标记为“已处理”吗？\n平台账号：${row.billing_username}\n\n建议在节点账号开通完成后再标记。`,
      "标记已处理",
      { type: "warning", confirmButtonText: "确认标记", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  openRequestActionLoadingId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminApproveRequest(requestID);
    success.value = `申请 ${requestID} 已标记为已处理`;
    await reloadOpenRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestActionLoadingId.value = 0;
  }
}

async function markOpenRequestPending(row: UserRequest) {
  const requestID = Number(row.request_id || 0);
  if (!requestID || String(row.status || "").trim() === "pending") return;
  try {
    await ElMessageBox.confirm(
      `确认将申请 ${requestID} 恢复为“待处理”吗？\n平台账号：${row.billing_username}\n当前状态：${requestStatusText(row.status)}\n\n适用于误点“标记已处理”的情况。`,
      "恢复待处理",
      { type: "warning", confirmButtonText: "确认恢复", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  openRequestActionLoadingId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminReopenRequest(requestID);
    success.value = `申请 ${requestID} 已恢复为待处理`;
    await reloadOpenRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestActionLoadingId.value = 0;
  }
}

async function markOpenRequestRejected(row: UserRequest) {
  const requestID = Number(row.request_id || 0);
  if (!requestID || String(row.status || "").trim() !== "pending") return;
  let reason = "";
  try {
    const input: any = await ElMessageBox.prompt(
      `请填写拒绝理由（建议必填，便于历史追踪）：\n平台账号：${row.billing_username}\n申请类型：${requestTypeText(row.request_type)}\n节点：${row.node_id || "-"}\n节点账号：${row.local_username || "-"}`,
      "拒绝申请",
      {
        type: "warning",
        confirmButtonText: "确认拒绝",
        cancelButtonText: "取消",
        inputType: "textarea",
        inputPlaceholder: "例如：申请信息不完整，请补充后重提",
      },
    );
    reason = String(input?.value || "").trim();
  } catch {
    return;
  }
  openRequestActionLoadingId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminRejectRequest(requestID, reason);
    success.value = `申请 ${requestID} 已拒绝`;
    await reloadOpenRequests();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    openRequestActionLoadingId.value = 0;
  }
}

function canReviewUnbindRecord(row: UserNodeUnbindRecord): boolean {
  const requestID = Number(row.request_id || 0);
  const status = String(row.status || "").trim();
  const sourceType = String(row.source_type || "").trim();
  return requestID > 0 && status === "pending" && sourceType === "user_request";
}

async function approveUnbindRecord(row: UserNodeUnbindRecord) {
  const requestID = Number(row.request_id || 0);
  if (!canReviewUnbindRecord(row) || requestID <= 0) return;
  try {
    await ElMessageBox.confirm(
      `确认通过解绑申请 ${requestID} 吗？\n平台账号：${row.billing_username}\n节点：${row.node_id}\n节点账号：${row.local_username}`,
      "通过解绑申请",
      { type: "warning", confirmButtonText: "确认通过", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  unbindRecordActionLoadingRequestId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminApproveRequest(requestID);
    success.value = `解绑申请 ${requestID} 已通过`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    unbindRecordActionLoadingRequestId.value = 0;
  }
}

async function rejectUnbindRecord(row: UserNodeUnbindRecord) {
  const requestID = Number(row.request_id || 0);
  if (!canReviewUnbindRecord(row) || requestID <= 0) return;
  let reason = "";
  try {
    const input: any = await ElMessageBox.prompt(
      `请填写拒绝理由（必填）：\n平台账号：${row.billing_username}\n节点：${row.node_id}\n节点账号：${row.local_username}`,
      "拒绝解绑申请",
      {
        type: "warning",
        confirmButtonText: "确认拒绝",
        cancelButtonText: "取消",
        inputType: "textarea",
        inputPlaceholder: "例如：未核验身份材料，请补充后重新提交",
        inputValidator: (v: string) => String(v || "").trim().length > 0 || "拒绝理由不能为空",
      },
    );
    reason = String(input?.value || "").trim();
  } catch {
    return;
  }
  if (!reason) {
    ElMessage.warning("拒绝理由不能为空");
    return;
  }
  unbindRecordActionLoadingRequestId.value = requestID;
  error.value = "";
  success.value = "";
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminRejectRequest(requestID, reason);
    success.value = `解绑申请 ${requestID} 已拒绝`;
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    unbindRecordActionLoadingRequestId.value = 0;
  }
}

async function save() {
  error.value = "";
  success.value = "";
  const billing = formBilling.value.trim();
  const node = nodeId.value.trim();
  const local = localUsername.value.trim();
  if (!billing || !node || !local) {
    error.value = "平台账号、节点编号、节点账号均不能为空";
    return;
  }
  saving.value = true;
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    if (editMode.value === "edit" && old.value) {
      await client.adminUpdateAccount({
        old_billing_username: old.value.billing,
        old_node_id: old.value.node,
        old_local_username: old.value.local,
        new_billing_username: billing,
        new_node_id: node,
        new_local_username: local,
      });
      success.value = "修改成功";
    } else {
      await client.adminUpsertAccount({
        billing_username: billing,
        node_id: node,
        local_username: local,
      });
      success.value = "新增成功";
    }
    editVisible.value = false;
    clearEditForm();
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  } finally {
    saving.value = false;
  }
}

async function submitUnbindRequest(row: UserNodeAccount) {
  error.value = "";
  success.value = "";
  let reason = "";
  try {
    const input: any = await ElMessageBox.prompt(
      `请填写代提交解绑申请理由（至少 10 个字）：\n平台账号：${row.billing_username}\n节点：${row.node_id}\n节点账号：${row.local_username}`,
      "代提交解绑申请",
      {
        type: "warning",
        confirmButtonText: "提交申请",
        cancelButtonText: "取消",
        inputPlaceholder: "例如：管理员核查确认停用该映射",
      },
    );
    reason = String(input?.value || "").trim();
    if (reason.length < 10) {
      ElMessage.warning("解绑理由至少 10 个字");
      return;
    }
  } catch {
    return;
  }
  try {
    await ElMessageBox.confirm(
      `确认代平台账号 ${row.billing_username} 提交解绑申请吗？\n节点：${row.node_id}\n节点账号：${row.local_username}`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认提交", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    const r = await client.adminCreateUnbindRequest({
      billing_username: row.billing_username,
      node_id: row.node_id,
      local_username: row.local_username,
      reason,
    });
    success.value = `已代提交解绑申请（ID ${r.request_id}），等待审批`;
    await Promise.all([reloadOpenRequests(), reloadUnbindRecords()]);
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function remove(row: UserNodeAccount) {
  error.value = "";
  success.value = "";
  let reason = "";
  try {
    const input: any = await ElMessageBox.prompt(
      `请填写强制解绑理由（至少 10 个字）：\n平台账号：${row.billing_username}\n节点：${row.node_id}\n节点账号：${row.local_username}`,
      "强制解绑",
      {
        type: "warning",
        confirmButtonText: "下一步",
        cancelButtonText: "取消",
        inputPlaceholder: "例如：违规使用、账号停用、归属变更",
      },
    );
    reason = String(input?.value || "").trim();
    if (reason.length < 10) {
      ElMessage.warning("解绑理由至少 10 个字");
      return;
    }
  } catch {
    return;
  }
  try {
    await ElMessageBox.confirm(
      `你正在执行强制解绑：\n平台账号：${row.billing_username}\n节点：${row.node_id}\n节点账号：${row.local_username}\n\n该操作会立即终止进程并断开 SSH。`,
      "第一次确认",
      { type: "warning", confirmButtonText: "继续", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    await ElMessageBox.confirm(
      `最后确认：是否强制解绑 ${row.node_id}/${row.local_username} ？\n\n理由：${reason}`,
      "第二次确认",
      { type: "warning", confirmButtonText: "确认强制解绑", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  try {
    const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
    await client.adminDeleteAccount({
      billing_username: row.billing_username,
      node_id: row.node_id,
      local_username: row.local_username,
      reason,
    });
    success.value = "已强制解绑，记录已写入解绑记录";
    await reload();
  } catch (e: any) {
    error.value = e?.message ?? String(e);
  }
}

async function copyText(text: string) {
  const value = String(text || "").trim();
  if (!value) return;
  try {
    await writeClipboardText(value);
    ElMessage.success("已复制");
  } catch {
    ElMessage.error("复制失败，请手动复制");
  }
}

async function provisionAccount() {
  provisionActionError.value = "";
  provisionActionSuccess.value = "";
  const billing = provisionForm.billing_username.trim();
  const node = provisionForm.node_id.trim();
  const local = provisionForm.local_username.trim();
  if (!billing || !node || !local) {
    provisionActionError.value = "平台账号、节点编号、节点账号不能为空";
    await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  await fetchProvisionUserDetail(false);
  if (!provisionUserDetail.value || String(provisionUserDetail.value.username || "").trim() !== billing) {
    provisionActionError.value = provisionUserError.value || "请先确认平台账号详细信息，确认无误后再开通";
    await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  if (!/^[a-z_][a-z0-9_-]{0,31}$/.test(local)) {
    provisionActionError.value = "节点账号格式不合法：需以小写字母或下划线开头，只能包含小写字母、数字、下划线、短横线";
    await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    return;
  }
  const sshHost = provisionForm.ssh_host.trim();
  const sshPort = Number(provisionForm.ssh_port || 0);
  rememberProvisionSSHHost(sshHost);
  try {
    await ElMessageBox.confirm(
      `请确认本次开通绑定信息：\n平台账号：${billing}\n节点编号：${node}\n节点账号：${local}\n\n确认后将生成密钥并发送邮件，绑定错误会影响用户登录。`,
      "二次确认",
      { type: "warning", confirmButtonText: "确认开通并绑定", cancelButtonText: "取消" },
    );
  } catch {
    return;
  }
  const client = new ApiClient(settingsState.baseUrl, { csrfToken: authState.csrfToken });
  const applyProvisionSuccess = async (r: AdminAccountProvisionResp, forceReissued = false) => {
    provisionResult.value = r;
    provisionResultVisible.value = true;
    const isReissued = !!r.reissued_key || forceReissued;
    if (r.mail_sent) {
      provisionActionSuccess.value = isReissued
        ? `新密钥已重新生成；密文已发平台内通知，提取码已发送到 ${r.email}`
        : `节点账号已开通；密文已发平台内通知，提取码已发送到 ${r.email}`;
    } else {
      provisionActionSuccess.value = isReissued
        ? "新密钥已重新生成，平台内密文通知已生成，但提取码邮件发送失败"
        : "节点账号已开通，平台内密文通知已生成，但提取码邮件发送失败";
    }
    await reload();
    await reloadProvisionLogs();
  };

  const parseProvisionConflict = (e: any): { shouldOfferRotate: boolean; mappedBilling: string } => {
    const status = Number(e?.status || 0);
    if (status !== 409) return { shouldOfferRotate: false, mappedBilling: "" };
    const msg = String(e?.message || "").trim();
    let reason = "";
    let mappedBilling = "";
    const bodyText = String(e?.body || "").trim();
    if (bodyText) {
      try {
        const j = JSON.parse(bodyText);
        reason = String(j?.reason || "").trim();
        mappedBilling = String(j?.mapped_billing_username || "").trim();
      } catch {
        // ignore parse failure and fallback to message matching
      }
    }
    if (reason === "mapping_exists_other_user") {
      return { shouldOfferRotate: false, mappedBilling };
    }
    const sameUserReason = reason === "mapping_exists_same_user";
    const compatibleLegacyReason = reason === "mapping_exists" && (!mappedBilling || mappedBilling === billing);
    const legacyMsgHint = !reason && (msg.includes("已有平台映射") || msg.includes("已绑定到平台账号"));
    return {
      shouldOfferRotate: sameUserReason || compatibleLegacyReason || legacyMsgHint,
      mappedBilling,
    };
  };

  provisioning.value = true;
  try {
    const r = await client.adminProvisionAccount({
      billing_username: billing,
      node_id: node,
      local_username: local,
      ssh_host: sshHost || undefined,
      ssh_port: sshPort > 0 ? sshPort : undefined,
    });
    await applyProvisionSuccess(r, false);
  } catch (e: any) {
    const conflict = parseProvisionConflict(e);
    if (!conflict.shouldOfferRotate) {
      provisionActionError.value = e?.message ?? String(e);
      await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
      return;
    }
    const mapped = conflict.mappedBilling || billing;
    try {
      await ElMessageBox.confirm(
        `检测到该节点账号已存在映射。\n节点编号：${node}\n节点账号：${local}\n平台账号：${mapped}\n\n该用户可能是丢失了旧密钥。是否立即重新生成新密钥，并重新发送“平台内密文 + 邮件提取码”？`,
        "二次提醒：是否重发新密钥",
        { type: "warning", confirmButtonText: "重新生成并重发", cancelButtonText: "取消" },
      );
    } catch {
      return;
    }
    try {
      const r2 = await client.adminProvisionAccount({
        billing_username: billing,
        node_id: node,
        local_username: local,
        ssh_host: sshHost || undefined,
        ssh_port: sshPort > 0 ? sshPort : undefined,
        rotate_key: true,
      });
      await applyProvisionSuccess(r2, true);
    } catch (e2: any) {
      provisionActionError.value = e2?.message ?? String(e2);
      await ElMessageBox.alert(provisionActionError.value, "开通失败", { type: "error", confirmButtonText: "我知道了" });
    }
  } finally {
    provisioning.value = false;
  }
}

async function init() {
  try {
    await loadPlatformUsers();
  } catch {
    platformUsers.value = [];
  }
  try {
    await loadNodeOptions();
  } catch {
    nodeOptions.value = [];
    nodeIPByID.value = {};
  }
  try {
    await reloadBindPolicy();
  } catch {
    // ignore
  }
  await reload();
}

init();
</script>

<style scoped>
.admin-accounts-page {
  display: flex;
  flex-direction: column;
  gap: 12px;
}

.section-card {
  border: 1px solid var(--border-color);
  box-shadow: 0 6px 18px rgba(15, 23, 42, 0.05);
}

.section-card :deep(.el-card__header) {
  padding: 12px 16px;
  background: #f7fbff;
  border-bottom: 1px solid var(--border-color);
}
.overview-card { order: 1; }
.mapping-query-card { order: 2; }
.mapping-list-card { order: 3; }
.restricted-card { order: 4; }
.mapping-review-card { order: 5; }
.unbind-records-card { order: 6; }
.provision-card { order: 7; }
.provision-history-card { order: 8; }
.bind-policy-card { order: 9; }
.risk-card { order: 10; }

.head { display: flex; justify-content: space-between; align-items: center; gap: 10px; }
.head-actions { display: flex; gap: 8px; }
.section-title-wrap {
  display: inline-flex;
  align-items: center;
  gap: 10px;
  font-weight: 700;
  color: var(--text-primary);
}
.section-icon {
  width: 30px;
  height: 30px;
  display: inline-flex;
  align-items: center;
  justify-content: center;
  border-radius: 9px;
  box-shadow:
    inset 0 0 0 1px rgba(255, 255, 255, 0.35),
    0 4px 12px rgba(15, 23, 42, 0.15);
}
.section-icon :deep(svg) {
  width: 17px;
  height: 17px;
}
.tone-map {
  background: linear-gradient(135deg, #0f766e, #0d9488);
  color: #ccfbf1;
}
.tone-provision {
  background: linear-gradient(135deg, #4f46e5, #4338ca);
  color: #e0e7ff;
}
.tone-history {
  background: linear-gradient(135deg, #b45309, #d97706);
  color: #fef3c7;
}
.tone-query {
  background: linear-gradient(135deg, #0369a1, #0284c7);
  color: #e0f2fe;
}
.tone-list {
  background: linear-gradient(135deg, #be123c, #e11d48);
  color: #ffe4e6;
}
.tone-note {
  background: linear-gradient(135deg, #0369a1, #0ea5e9);
  color: #e0f2fe;
}
.tone-risk {
  background: linear-gradient(135deg, #b91c1c, #ef4444);
  color: #fee2e2;
}
.tone-user {
  background: linear-gradient(135deg, #1d4ed8, #2563eb);
  color: #dbeafe;
}
.mb { margin-bottom: 12px; }
.policy-label {
  display: inline-flex;
  align-items: center;
  gap: 6px;
}
.policy-help-icon {
  color: #2563eb;
  cursor: help;
  font-size: 13px;
}
.policy-help-tip {
  max-width: 340px;
  line-height: 1.45;
}
.risk-card :deep(.el-card__body) {
  padding-top: 12px;
}
.risk-users {
  display: flex;
  flex-wrap: wrap;
  gap: 8px;
}
.switch-history-wrap {
  display: flex;
  flex-direction: column;
  gap: 4px;
}
.switch-history-item {
  display: inline-flex;
  align-items: center;
  color: #991b1b;
  font-size: 12px;
  background: rgba(254, 226, 226, 0.65);
  border: 1px solid rgba(248, 113, 113, 0.3);
  border-radius: 7px;
  padding: 2px 8px;
}
.risk-user-item {
  display: inline-flex;
  align-items: center;
  gap: 6px;
  background: rgba(248, 113, 113, 0.08);
  border: 1px solid rgba(248, 113, 113, 0.28);
  border-radius: 8px;
  padding: 2px 8px;
}
.risk-dot {
  display: inline-block;
  width: 9px;
  height: 9px;
  border-radius: 999px;
  background: #ef4444;
  box-shadow: 0 0 0 3px rgba(239, 68, 68, 0.18);
}
.risk-dot.mini {
  width: 7px;
  height: 7px;
  margin-right: 6px;
}
.map-user-cell {
  display: inline-flex;
  align-items: center;
}
.provision-card :deep(.el-card__body) { padding-top: 14px; }
.provision-head { font-weight: 700; }
.provision-user-preview {
  margin: 8px 0 12px;
  padding: 10px 12px;
  border: 1px solid var(--border-color);
  border-radius: 10px;
  background: linear-gradient(180deg, rgba(148, 163, 184, 0.08), rgba(148, 163, 184, 0.02));
}
.preview-head {
  display: flex;
  justify-content: space-between;
  align-items: center;
  gap: 10px;
  margin-bottom: 8px;
}
.preview-desc :deep(.el-descriptions__label) {
  min-width: 90px;
}
.provision-actions { display: flex; gap: 8px; flex-wrap: wrap; }
.query-form {
  margin-bottom: -2px;
}
.restricted-details {
  display: flex;
  flex-direction: column;
  gap: 2px;
  color: #475569;
  font-size: 12px;
}
.kv-row {
  margin-top: 12px;
  display: grid;
  grid-template-columns: 90px 1fr auto;
  gap: 8px;
  align-items: center;
}
.kv-label { color: #475569; font-size: 13px; }
.payload-actions {
  margin-top: 8px;
  display: flex;
  gap: 8px;
}
.note-user-detail {
  margin: 4px 0;
}
.note-user-lines {
  display: flex;
  flex-direction: column;
  gap: 2px;
}
.note-user-lines .line-1 {
  color: #0f172a;
  font-weight: 600;
}
.note-user-lines .line-2,
.note-user-lines .line-3 {
  color: #475569;
  font-size: 12px;
}
.pending-count-badge {
  margin-left: 2px;
}
.pending-count-badge :deep(.el-badge__content) {
  font-weight: 700;
}
.pending-open-banner {
  border: 1px solid #f59e0b;
  background: linear-gradient(180deg, rgba(245, 158, 11, 0.12), rgba(245, 158, 11, 0.04));
}
.pending-open-actions {
  margin-top: 6px;
  display: flex;
  gap: 8px;
  flex-wrap: wrap;
}

@media (max-width: 900px) {
  .head {
    flex-wrap: wrap;
  }
}
</style>
