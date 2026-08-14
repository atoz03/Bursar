<template>
  <div class="demo-page">
    <section class="ops-page-hero demo-hero">
      <div class="ops-hero-copy">
        <span class="ops-eyebrow">INTERFACE SANDBOX</span>
        <div class="ops-title-row">
          <span class="ops-hero-icon"><el-icon><View /></el-icon></span>
          <div>
            <h1>界面演示</h1>
            <p>用本地 Mock 数据预览不同身份下的页面，不连接业务接口。</p>
          </div>
        </div>
      </div>
      <div class="ops-hero-actions">
        <el-tag type="warning" effect="dark">MOCK ONLY</el-tag>
        <span class="demo-route">{{ demoPath }}</span>
      </div>
    </section>

    <el-card class="demo-controls" shadow="never">
      <div class="control-row">
        <div class="control-block">
          <span class="control-label">模拟身份</span>
          <el-radio-group v-model="activeRole">
            <el-radio-button v-for="role in roleOptions" :key="role.value" :label="role.value">
              {{ role.label }}
            </el-radio-button>
          </el-radio-group>
        </div>
        <div class="control-block page-control">
          <span class="control-label">预览页面</span>
          <el-select v-model="activePageId" filterable>
            <el-option v-for="page in availablePages" :key="page.id" :label="page.title" :value="page.id" />
          </el-select>
        </div>
      </div>
    </el-card>

    <section class="preview-window">
      <header class="preview-window-head">
        <div class="window-dots"><i /><i /><i /></div>
        <div class="window-address">demo://{{ activeRole }}/{{ activePage.id }}</div>
        <el-tag size="small" type="info" effect="plain">{{ activeRoleLabel }}</el-tag>
      </header>

      <div class="preview-workspace" :class="{ 'guest-workspace': activeRole === 'guest' }">
        <aside v-if="activeRole !== 'guest'" class="preview-sidebar">
          <div class="preview-brand">
            <span><el-icon><Cpu /></el-icon></span>
            <div><b>GPU Ops</b><small>PREVIEW</small></div>
          </div>
          <button
            v-for="page in availablePages"
            :key="page.id"
            type="button"
            :class="{ active: page.id === activePageId }"
            @click="activePageId = page.id"
          >
            <i />{{ page.title }}
          </button>
        </aside>

        <main class="preview-canvas" :class="`kind-${activePage.kind}`">
          <template v-if="activePage.kind === 'login'">
            <div class="auth-preview">
              <section class="auth-intro">
                <el-tag effect="dark" type="success">HIT AIOT LAB</el-tag>
                <h2>GPU / CPU<br />集群管理平台</h2>
                <p>统一管理计算资源、节点账号与积分。</p>
              </section>
              <section class="auth-panel">
                <h2>欢迎登录</h2>
                <p>演示账号不会提交到服务器</p>
                <el-form label-position="top">
                  <el-form-item label="用户名"><el-input v-model="mockForm.username" /></el-form-item>
                  <el-form-item label="密码"><el-input v-model="mockForm.password" type="password" show-password /></el-form-item>
                  <el-button type="primary" class="full-button" @click="mockAction">登录演示</el-button>
                </el-form>
              </section>
            </div>
          </template>

          <template v-else-if="activePage.kind === 'register'">
            <div class="mock-page-head compact-head">
              <div><span>ACCOUNT APPLICATION</span><h2>注册申请</h2><p>填写身份资料，提交后等待管理员审核。</p></div>
              <el-tag type="success" effect="plain">邮箱验证</el-tag>
            </div>
            <el-card class="mock-form-card" shadow="never">
              <div class="section-caption">基本资料</div>
              <el-form :model="mockForm" label-position="top">
                <el-row :gutter="16">
                  <el-col :span="12"><el-form-item label="真实姓名"><el-input v-model="mockForm.realName" /></el-form-item></el-col>
                  <el-col :span="12"><el-form-item label="学号"><el-input v-model="mockForm.studentId" /></el-form-item></el-col>
                  <el-col :span="12"><el-form-item label="邮箱"><el-input v-model="mockForm.email" /></el-form-item></el-col>
                  <el-col :span="12"><el-form-item label="导师"><el-input v-model="mockForm.advisor" /></el-form-item></el-col>
                </el-row>
                <el-alert title="这是演示表单，不会发送验证码或创建申请。" type="info" :closable="false" show-icon />
                <div class="form-actions"><el-button type="primary" @click="mockAction">提交注册申请</el-button></div>
              </el-form>
            </el-card>
          </template>

          <template v-else>
            <div class="mock-page-head">
              <div>
                <span>{{ activePage.eyebrow }}</span>
                <h2>{{ activePage.title }}</h2>
                <p>{{ activePage.description }}</p>
              </div>
              <div class="mock-head-actions">
                <el-tag type="success" effect="plain">Mock 数据</el-tag>
                <el-button type="primary" @click="mockAction">{{ activePage.action }}</el-button>
              </div>
            </div>

            <div v-if="showMetrics" class="mock-metrics">
              <article v-for="metric in activeMetrics" :key="metric.label" :class="metric.tone">
                <span>{{ metric.label }}</span><strong>{{ metric.value }}</strong><small>{{ metric.note }}</small>
              </article>
            </div>

            <div v-if="activePage.kind === 'cluster'" class="mock-node-grid">
              <article v-for="node in mockNodes" :key="node.id" class="mock-node-card">
                <header><div><i :class="node.state" /><b>{{ node.id }}</b></div><el-tag size="small" :type="node.online ? 'success' : 'info'">{{ node.online ? '在线' : '离线' }}</el-tag></header>
                <div class="node-spec">{{ node.cpu }} · {{ node.gpus.length }} 张 GPU</div>
                <div class="node-meter"><span>CPU <b>{{ node.cpuUse }}%</b></span><em><i :style="{ width: `${node.cpuUse}%` }" /></em></div>
                <div class="gpu-demo-grid">
                  <div v-for="gpu in node.gpus" :key="gpu.name"><span>{{ gpu.name }}</span><b>{{ gpu.use }}%</b><em><i :style="{ width: `${gpu.use}%` }" /></em></div>
                </div>
              </article>
            </div>

            <el-card v-else-if="activePage.kind === 'form' || activePage.kind === 'profile'" class="mock-form-card" shadow="never">
              <div class="section-caption">{{ activePage.section }}</div>
              <el-form :model="mockForm" label-width="116px">
                <el-form-item label="功能开关"><el-switch v-model="mockForm.enabled" inline-prompt active-text="开" inactive-text="关" /></el-form-item>
                <el-form-item label="名称"><el-input v-model="mockForm.title" /></el-form-item>
                <el-form-item label="通知邮箱"><el-input v-model="mockForm.email" /></el-form-item>
                <el-form-item label="说明"><el-input v-model="mockForm.note" type="textarea" :rows="3" /></el-form-item>
                <el-form-item><el-button type="primary" @click="mockAction">保存演示设置</el-button></el-form-item>
              </el-form>
            </el-card>

            <el-card v-else-if="activePage.kind === 'document' || activePage.kind === 'notices'" class="mock-document" shadow="never">
              <div class="document-meta"><el-tag size="small">已发布</el-tag><span>更新于 2026-08-14 14:30</span></div>
              <h3>{{ activePage.title }}示例</h3>
              <p>这里展示该页面的排版、信息层级和操作区域。演示内容完全保存在当前浏览器内存中。</p>
              <el-alert title="维护窗口：本周六 02:00—03:00，期间部分节点可能短暂离线。" type="warning" :closable="false" show-icon />
            </el-card>

            <el-card v-else class="mock-table-card" shadow="never">
              <div class="table-toolbar">
                <el-input v-model="keyword" clearable placeholder="搜索 Mock 数据" />
                <el-button @click="mockAction">筛选</el-button>
              </div>
              <el-table :data="filteredRows" stripe height="310" empty-text="暂无 Mock 数据">
                <el-table-column prop="name" :label="activePage.primaryColumn" min-width="150" />
                <el-table-column prop="identity" label="账号 / 节点" min-width="140" />
                <el-table-column prop="resource" label="资源与积分" min-width="160" />
                <el-table-column prop="status" label="状态" width="100">
                  <template #default="{ row }"><el-tag size="small" :type="row.status === '正常' ? 'success' : 'warning'">{{ row.status }}</el-tag></template>
                </el-table-column>
                <el-table-column prop="updated" label="更新时间" width="170" />
                <el-table-column label="操作" width="150" fixed="right">
                  <template #default><el-button size="small" @click="mockAction">查看</el-button><el-button size="small" type="primary" @click="mockAction">处理</el-button></template>
                </el-table-column>
              </el-table>
            </el-card>
          </template>
        </main>
      </div>
    </section>
  </div>
</template>

<script setup lang="ts">
import { computed, reactive, ref, watch } from "vue";
import { useRoute, useRouter } from "vue-router";
import { ElMessage } from "element-plus";
import { Cpu, View } from "@element-plus/icons-vue";

type DemoRole = "admin" | "power_user" | "user" | "guest";
type DemoKind = "dashboard" | "cluster" | "table" | "points" | "form" | "profile" | "document" | "notices" | "login" | "register";
type DemoPage = {
  id: string;
  title: string;
  description: string;
  eyebrow: string;
  kind: DemoKind;
  action: string;
  section: string;
  primaryColumn: string;
};

const route = useRoute();
const router = useRouter();
const roleOptions: Array<{ value: DemoRole; label: string }> = [
  { value: "admin", label: "管理员" },
  { value: "power_user", label: "高级用户" },
  { value: "user", label: "普通用户" },
  { value: "guest", label: "访客 / 注册" },
];

function page(id: string, title: string, kind: DemoKind, description: string, action = "刷新", primaryColumn = "名称", section = "配置内容"): DemoPage {
  return { id, title, kind, description, action, primaryColumn, section, eyebrow: id.replaceAll("-", " ").toUpperCase() };
}

const pageCatalog: Record<DemoRole, DemoPage[]> = {
  admin: [
    page("board", "运营看板", "dashboard", "资源使用、积分变化和活跃用户概览。", "同步数据"),
    page("status", "集群状态", "cluster", "CPU 与多 GPU 实时状态卡片。", "刷新状态"),
    page("nodes", "节点管理", "table", "节点策略、版本与运行状态。", "同步节点", "节点"),
    page("usage", "进程审计", "table", "按用户和节点查看进程记录。", "导出 CSV", "进程"),
    page("points", "积分管理", "points", "积分余额、规则和调整记录。", "调整积分", "用户"),
    page("users", "平台用户", "table", "平台账号、身份信息和状态。", "导出 CSV", "平台账号"),
    page("accounts", "账号映射", "table", "平台账号与节点账号绑定关系。", "新增映射", "映射关系"),
    page("provision", "账号开通", "form", "节点账号开通与密钥下发。", "开通账号", "节点账号开通"),
    page("requests", "注册与资料审核", "table", "注册、资料修改和解绑申请。", "批量审核", "申请人"),
    page("power-users", "高级用户", "table", "授权范围与管理权限。", "新增授权", "高级用户"),
    page("whitelist", "SSH 名单", "table", "黑白名单与临时账号。", "新增规则", "账号"),
    page("ha", "容灾同步", "form", "主备同步状态和容灾配置。", "检查同步", "容灾设置"),
    page("announcements", "公告管理", "notices", "平台公告的发布与维护。", "新建公告"),
    page("guideline", "用户准则", "document", "注册与资源使用规范。", "编辑准则"),
    page("notebook", "管理员记事本", "document", "内部运维记录与交接信息。", "新建记录"),
    page("mail", "邮件设置", "form", "SMTP、模板和通知策略。", "发送测试", "邮件服务"),
    page("profile", "管理员资料", "profile", "管理员资料与安全状态。", "保存资料", "个人资料"),
  ],
  power_user: [
    page("board", "运营看板", "dashboard", "已授权范围内的运营数据。", "同步数据"),
    page("nodes", "节点管理", "table", "已授权节点的状态与策略。", "刷新", "节点"),
    page("points", "积分管理", "points", "按授权查看和调整用户积分。", "调整积分", "用户"),
    page("requests", "注册审核", "table", "待审核注册与资料变更。", "审核", "申请人"),
    page("balance", "我的积分", "points", "个人可用积分与变动。", "刷新", "积分记录"),
    page("accounts", "节点账号", "table", "个人节点账号与开通进度。", "申请开通", "节点账号"),
  ],
  user: [
    page("balance", "我的积分", "points", "通用、结转和节点专属积分。", "刷新", "积分记录"),
    page("usage", "我的用量", "dashboard", "CPU、GPU 使用时间和积分消耗。", "查询"),
    page("accounts", "节点账号", "table", "节点账号、登录状态与开通申请。", "申请开通", "节点账号"),
    page("notices", "公告与准则", "notices", "平台公告和使用规范。", "标记已读"),
    page("profile", "个人资料", "profile", "身份资料与安全设置。", "保存资料", "个人资料"),
  ],
  guest: [
    page("login", "登录", "login", "账号登录与双因素验证。", "登录"),
    page("register", "注册申请", "register", "新用户注册、邮箱验证和资料填写。", "提交申请"),
    page("forgot-password", "找回密码", "form", "通过邮箱重置登录密码。", "发送邮件", "身份验证"),
  ],
};

function queryText(value: unknown): string {
  return Array.isArray(value) ? String(value[0] || "") : String(value || "");
}

const queryRole = queryText(route.query.role) as DemoRole;
const initialRole: DemoRole = roleOptions.some((item) => item.value === queryRole) ? queryRole : "admin";
const activeRole = ref<DemoRole>(initialRole);
const initialPage = queryText(route.query.page);
const activePageId = ref(pageCatalog[initialRole].some((item) => item.id === initialPage) ? initialPage : pageCatalog[initialRole][0].id);
const availablePages = computed(() => pageCatalog[activeRole.value]);
const activePage = computed(() => availablePages.value.find((item) => item.id === activePageId.value) || availablePages.value[0]);
const activeRoleLabel = computed(() => roleOptions.find((item) => item.value === activeRole.value)?.label || "管理员");
const demoPath = computed(() => `/admin/demo?role=${activeRole.value}&page=${activePage.value.id}`);
const keyword = ref("");
const mockForm = reactive({
  username: "demo-user",
  password: "preview-only",
  realName: "演示用户",
  studentId: "20260001",
  email: "demo@example.org",
  advisor: "示例导师",
  enabled: true,
  title: "HIT AIOT 演示配置",
  note: "此处内容仅用于预览页面排版。",
});

const mockRows = [
  { name: "示例记录 A", identity: "60020 / demo-a", resource: "GPU 2 · 468.20", status: "正常", updated: "2026-08-14 15:20" },
  { name: "示例记录 B", identity: "60021 / demo-b", resource: "CPU 64 · 320.00", status: "待处理", updated: "2026-08-14 15:16" },
  { name: "示例记录 C", identity: "60022 / demo-c", resource: "GPU 4 · 126.50", status: "正常", updated: "2026-08-14 15:08" },
  { name: "示例记录 D", identity: "60018 / demo-d", resource: "CPU 192 · 80.00", status: "正常", updated: "2026-08-14 14:57" },
];
const filteredRows = computed(() => {
  const value = keyword.value.trim().toLowerCase();
  if (!value) return mockRows;
  return mockRows.filter((row) => Object.values(row).join(" ").toLowerCase().includes(value));
});

const mockNodes = [
  { id: "60020", online: true, state: "good", cpu: "Intel Xeon 64C", cpuUse: 31, gpus: [{ name: "GPU 0", use: 72 }, { name: "GPU 1", use: 18 }, { name: "GPU 2", use: 0 }, { name: "GPU 3", use: 44 }] },
  { id: "60021", online: true, state: "warn", cpu: "AMD EPYC 96C", cpuUse: 68, gpus: [{ name: "GPU 0", use: 91 }, { name: "GPU 1", use: 87 }] },
  { id: "60022", online: false, state: "muted", cpu: "AMD EPYC 64C", cpuUse: 0, gpus: [{ name: "GPU 0", use: 0 }, { name: "GPU 1", use: 0 }] },
];

const showMetrics = computed(() => ["dashboard", "points", "table"].includes(activePage.value.kind));
const activeMetrics = computed(() => activePage.value.kind === "points"
  ? [
      { label: "总积分", value: "2,840.50", note: "全部可用余额", tone: "blue" },
      { label: "通用积分", value: "1,920.50", note: "本月可用", tone: "green" },
      { label: "结转积分", value: "620.00", note: "历史结转", tone: "violet" },
      { label: "专属积分", value: "300.00", note: "节点限定", tone: "amber" },
    ]
  : [
      { label: "在线节点", value: "19 / 20", note: "95% 可用", tone: "green" },
      { label: "活跃 GPU", value: "11 / 82", note: "多卡状态", tone: "violet" },
      { label: "CPU 使用率", value: "32.8%", note: "集群平均", tone: "blue" },
      { label: "今日积分", value: "468.20", note: "累计消耗", tone: "amber" },
    ]);

watch(activeRole, () => {
  activePageId.value = pageCatalog[activeRole.value][0].id;
});
watch([activeRole, activePageId], () => {
  router.replace({ path: "/admin/demo", query: { role: activeRole.value, page: activePageId.value } });
});

function mockAction() {
  ElMessage.info("演示模式：该操作不会请求接口或修改真实数据");
}
</script>

<style scoped>
.demo-page { display: grid; gap: 16px; min-width: 0; }
.demo-route { color: #64748b; font-family: ui-monospace, SFMono-Regular, Menlo, monospace; font-size: 11px; }
.demo-controls :deep(.el-card__body) { padding: 14px 16px; }
.control-row { display: flex; align-items: end; gap: 20px; justify-content: space-between; }
.control-block { display: grid; gap: 7px; min-width: 0; }
.control-label { color: #64748b; font-size: 11px; font-weight: 800; letter-spacing: .08em; }
.page-control { width: min(310px, 100%); }

.preview-window { overflow: hidden; min-height: 680px; border: 1px solid rgba(255,255,255,.88); border-radius: 22px; background: rgba(255,255,255,.72); box-shadow: 0 22px 55px rgba(43,67,105,.14), inset 0 1px 0 #fff; }
.preview-window-head { height: 46px; display: grid; grid-template-columns: 100px 1fr auto; align-items: center; gap: 12px; padding: 0 16px; border-bottom: 1px solid #e7edf5; background: linear-gradient(180deg,#fff,#f5f8fc); }
.window-dots { display: flex; gap: 7px; }.window-dots i { width: 10px; height: 10px; border-radius: 50%; background: #fb7185; }.window-dots i:nth-child(2){background:#fbbf24}.window-dots i:nth-child(3){background:#34d399}
.window-address { justify-self: center; width: min(440px,100%); padding: 5px 12px; border: 1px solid #e2e8f0; border-radius: 8px; color: #8491a4; background:#f8fafc; font-size:11px; text-align:center; }
.preview-workspace { display:grid; grid-template-columns: 188px minmax(0,1fr); min-height: 634px; }.preview-workspace.guest-workspace{grid-template-columns:1fr}
.preview-sidebar { padding:14px 10px; overflow-y:auto; color:#dbeafe; background: linear-gradient(165deg,#111c31,#101827 58%,#0a1424); }
.preview-brand { display:flex; align-items:center; gap:9px; margin:0 4px 14px; padding:7px; }.preview-brand>span{width:30px;height:30px;display:grid;place-items:center;border-radius:9px;color:#fff;background:linear-gradient(135deg,#2563eb,#06b6d4)}.preview-brand div{display:grid}.preview-brand b{font-size:12px}.preview-brand small{color:#7890ad;font-size:8px;letter-spacing:.18em}
.preview-sidebar button { position:relative; width:100%; display:flex; align-items:center; gap:9px; margin:2px 0; padding:8px 10px; border:0; border-radius:9px; color:#9fb0c7; background:transparent; font:inherit; font-size:11px; text-align:left; cursor:pointer; }.preview-sidebar button i{width:5px;height:5px;border-radius:50%;background:#53657c}.preview-sidebar button:hover{color:#fff;background:rgba(255,255,255,.06)}.preview-sidebar button.active{color:#fff;background:linear-gradient(90deg,rgba(37,99,235,.82),rgba(59,130,246,.42));box-shadow:inset 0 1px 0 rgba(255,255,255,.15)}.preview-sidebar button.active i{background:#67e8f9;box-shadow:0 0 0 4px rgba(34,211,238,.12)}
.preview-canvas { min-width:0; padding:22px; overflow:hidden; background: radial-gradient(650px 330px at 95% 0%,rgba(129,140,248,.15),transparent 67%),linear-gradient(145deg,#eef6ff,#f8faff 52%,#eff9f8); }
.mock-page-head { display:flex; align-items:center; justify-content:space-between; gap:18px; margin-bottom:16px; padding:18px 20px; border:1px solid rgba(255,255,255,.9); border-radius:16px; background:linear-gradient(145deg,rgba(255,255,255,.9),rgba(245,249,255,.7)); box-shadow:0 12px 28px rgba(40,62,98,.08),inset 0 1px 0 #fff; }.mock-page-head span{color:#2563eb;font-size:9px;font-weight:850;letter-spacing:.14em}.mock-page-head h2{margin:3px 0 2px;color:#152033;font-size:25px;letter-spacing:-.03em}.mock-page-head p{margin:0;color:#718096;font-size:11px}.mock-head-actions{display:flex;align-items:center;gap:8px}.compact-head{margin-bottom:14px}
.mock-metrics { display:grid; grid-template-columns:repeat(4,minmax(0,1fr)); gap:10px; margin-bottom:14px; }.mock-metrics article{padding:14px;border:1px solid rgba(255,255,255,.9);border-top:3px solid currentColor;border-radius:13px;background:linear-gradient(145deg,rgba(255,255,255,.88),rgba(255,255,255,.58));box-shadow:0 8px 20px rgba(39,60,92,.07)}.mock-metrics span,.mock-metrics small{display:block;color:#7a899d;font-size:9px}.mock-metrics strong{display:block;margin:8px 0 5px;color:#182235;font-size:20px}.mock-metrics .blue{color:#3b82f6}.mock-metrics .green{color:#10b981}.mock-metrics .violet{color:#8b5cf6}.mock-metrics .amber{color:#f59e0b}
.mock-table-card,.mock-form-card,.mock-document{border-radius:16px!important}.table-toolbar{display:flex;gap:8px;margin-bottom:12px}.table-toolbar .el-input{max-width:280px}.section-caption{margin-bottom:14px;color:#26364b;font-size:14px;font-weight:800}.form-actions{display:flex;justify-content:flex-end;margin-top:14px}
.mock-node-grid{display:grid;grid-template-columns:repeat(3,minmax(0,1fr));gap:12px}.mock-node-card{padding:15px;border:1px solid rgba(255,255,255,.92);border-radius:15px;background:linear-gradient(145deg,rgba(255,255,255,.9),rgba(255,255,255,.58));box-shadow:0 10px 24px rgba(35,55,86,.08)}.mock-node-card header{display:flex;align-items:center;justify-content:space-between}.mock-node-card header>div{display:flex;align-items:center;gap:8px}.mock-node-card header i{width:8px;height:8px;border-radius:50%;background:#10b981}.mock-node-card header i.warn{background:#f59e0b}.mock-node-card header i.muted{background:#94a3b8}.node-spec{margin:7px 0 14px;color:#8491a2;font-size:9px}.node-meter span{display:flex;justify-content:space-between;color:#53647a;font-size:10px}.node-meter em,.gpu-demo-grid em{height:4px;display:block;margin-top:5px;overflow:hidden;border-radius:999px;background:#e5ebf2}.node-meter em i,.gpu-demo-grid em i{height:100%;display:block;border-radius:inherit;background:linear-gradient(90deg,#10b981,#34d399)}.gpu-demo-grid{display:grid;grid-template-columns:1fr 1fr;gap:7px;margin-top:12px}.gpu-demo-grid>div{padding:8px;border-radius:8px;background:rgba(248,250,252,.76);color:#66768b;font-size:8px}.gpu-demo-grid span{display:inline-block}.gpu-demo-grid b{float:right;color:#31445a}
.mock-document{min-height:280px}.mock-document h3{margin:18px 0 8px;font-size:22px}.mock-document p{max-width:720px;color:#64748b;line-height:1.75}.document-meta{display:flex;align-items:center;gap:10px;color:#94a3b8;font-size:10px}
.auth-preview{min-height:590px;display:grid;grid-template-columns:1.05fr .95fr;align-items:stretch;overflow:hidden;border-radius:20px;background:linear-gradient(130deg,#073b4c,#0f766e 48%,#d9f99d);box-shadow:0 20px 50px rgba(2,6,23,.16)}.auth-intro{display:flex;justify-content:center;flex-direction:column;padding:42px;color:#fff}.auth-intro .el-tag{align-self:flex-start}.auth-intro h2{margin:18px 0 8px;font-size:38px;line-height:1.14}.auth-intro p{font-size:15px;opacity:.88}.auth-panel{align-self:center;margin:28px;padding:28px;border:1px solid rgba(255,255,255,.9);border-radius:20px;background:linear-gradient(145deg,#fff,#eff8f7);box-shadow:0 18px 45px rgba(2,6,23,.18)}.auth-panel h2{margin:0;font-size:25px}.auth-panel>p{margin:5px 0 20px;color:#7a8798;font-size:11px}.full-button{width:100%}
@media(max-width:1000px){.preview-workspace{grid-template-columns:150px minmax(0,1fr)}.mock-metrics{grid-template-columns:repeat(2,1fr)}.mock-node-grid{grid-template-columns:1fr 1fr}}
@media(max-width:720px){.control-row{align-items:stretch;flex-direction:column}.page-control{width:100%}.preview-window-head{grid-template-columns:70px 1fr}.preview-window-head>.el-tag{display:none}.preview-workspace{grid-template-columns:1fr}.preview-sidebar{display:flex;gap:4px;overflow-x:auto}.preview-brand{display:none}.preview-sidebar button{width:auto;flex:0 0 auto}.preview-canvas{padding:12px}.mock-page-head{align-items:flex-start;flex-direction:column}.mock-metrics,.mock-node-grid{grid-template-columns:1fr}.auth-preview{grid-template-columns:1fr}.auth-intro{display:none}.auth-panel{margin:16px}.demo-controls :deep(.el-radio-group){display:grid;grid-template-columns:1fr 1fr}.demo-controls :deep(.el-radio-button__inner){width:100%}}
</style>
