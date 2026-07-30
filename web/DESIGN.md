# New API Default Web 设计系统

本文件是对现有默认主题和共享组件的最小记录，不引入新的视觉方向。依据为
`src/styles/theme.css`、`src/styles/theme-presets.css`、共享 UI 组件、现有
Risk Center 实现，以及 Issue #9 已通过的 375 / 768 / 1280 截图。

## 1. 氛围与识别

默认后台是克制、清晰、信息优先的运维控制台。主要识别来自主题可切换的单一主色、
轻量边框分层、紧凑但可读的排版，以及固定侧栏与可滚动主内容区；Risk Center 延续
现有页面壳和卡片层级，不增加装饰性图形、渐变、动画或新的视觉语言。

## 2. 颜色

所有产品界面颜色必须使用 `theme.css` 暴露的语义变量及其 Tailwind 映射：

| 角色     | 现有 token                                                       | 用途                             |
| -------- | ---------------------------------------------------------------- | -------------------------------- |
| 页面     | `--background` / `--foreground`                                  | 页面底色与正文                   |
| 卡片     | `--card` / `--card-foreground`                                   | 卡片、列表行                     |
| 浮层     | `--popover` / `--popover-foreground`                             | 下拉、提示、对话框               |
| 主操作   | `--primary` / `--primary-foreground`                             | 主要按钮、当前导航、焦点强调     |
| 次操作   | `--secondary` / `--secondary-foreground`                         | 次级操作                         |
| 弱化信息 | `--muted` / `--muted-foreground`                                 | 辅助底色、说明文字、空状态       |
| 交互强调 | `--accent` / `--accent-foreground`                               | hover、选中与侧栏强调            |
| 状态     | `--success`、`--warning`、`--destructive`、`--info`、`--neutral` | 安全、警告、错误、信息、中性状态 |
| 边界     | `--border`、`--input`、`--ring`                                  | 分隔线、输入边界、键盘焦点       |
| 表格     | `--table-row`、`--table-header`、`--table-header-hover`          | 表格和移动列表                   |
| 侧栏     | `--sidebar-*`                                                    | 应用导航壳                       |

- 亮暗模式和主题预设只通过现有变量覆盖；组件不写新的原始颜色。
- 主色仅用于真实操作、当前状态或焦点，不作装饰。
- `StatusBadge` 的 `success / warning / danger / info / neutral` 对应上述语义状态。

## 3. 排版

- 默认正文：`--font-body`，通常为 Public Sans；主题可切换到既有 Lora + CJK 回退栈。
- 页面标题：`SectionPageLayout.Title` 的 `text-base sm:text-lg`、粗体、紧密字距。
- 卡片标题：`TitledCard` 的 `text-lg sm:text-xl`；说明为 `text-xs sm:text-sm`。
- 正文、字段值与表格：`text-sm`；紧凑元数据可用现有 `text-xs`。
- 数据列使用 `tabular-nums`；请求标识等机器值可用现有 `font-mono`。
- 正文不低于现有 `text-sm`。移动端输入由全局样式固定为 16px，防止浏览器自动缩放。

## 4. 间距与布局

- 间距沿用 Tailwind 4 的现有比例，不新增另一套数值：紧凑 `gap-1/2`，常规
  `gap-3/4`，页面分区 `space-y-4`，卡片移动端 `p-3`、宽屏 `sm:p-5`。
- 页面壳使用 `SectionPageLayout`：标题/操作固定，内容区是唯一纵向滚动区域；内容区
  必须保留 `min-h-0`。
- 主内容宽度由应用侧栏壳和可选 `--max-content-width` 控制，页面不另设宽度系统。
- 375px 以单列呈现；768px 保持可读单列/自然换行；1280px 可使用现有双列网格。
- 表格只用于不会让 375px 主内容横向滚动的宽度；字段较多时在 `<640px` 改为
  单一带分隔线的行卡片列表。
- 长标识使用截断并保留 `title`；标签和类别允许换行；主内容不得依赖横向滚动。

## 5. 组件与状态

现有共享组件和 Issue #9 的真实页面/截图就是 primitive showcase；本任务不建立新的
展示路由。

### SectionPageLayout

- 结构：Title、Actions、Content、可选 Breadcrumb/Footer。
- 状态：操作区可换行；内容区可滚动；窄屏标题与操作自然换行。
- 可访问性：内容语义由页面子组件提供；操作使用真实按钮。

### TitledCard / Card

- 结构：可选图标、标题、说明、操作、带边框内容区。
- 表面：`bg-card`、语义 ring/border、现有 `rounded-xl`。
- 状态：默认、禁用 hover（运维信息卡默认使用）、加载、空、错误。

### Field

- 结构：label/title、description、control、error。
- 状态：默认、focus、disabled、invalid；错误区域使用 `role="alert"`。
- 可访问性：保留显式 label 和说明，不使用 placeholder 代替标签。

### StatusBadge

- 变体：`success`、`warning`、`danger`、`info`、`neutral`；形态为 badge/text/underline。
- 状态：默认、可复制 hover/active、可选 pulse；记录结果默认不可复制。
- 可访问性：状态必须同时有文本，不能只靠颜色。

### 表格与响应式行列表

- 宽屏：共享 `Table` / `StaticDataTable`，表头、分隔线、行 hover、等宽数字。
- 窄屏：复用 `MobileCardList` 的语法——单个有边框容器、行间 `divide-y`，而非多张
  漂浮卡片；首行显示主标识和结果，下面使用两列或单列字段。
- 状态：骨架加载、明确空状态、上下文错误及重试、分页按钮禁用状态。
- 风控结果和来源使用文本徽标；`error` 使用 warning，只有 `unsafe` 使用 destructive，`not_reviewed` /
  `local` 有明确语义，未来未知值以中性徽标原样回退，不能因契约扩展导致整页不可用。
- 风控延迟按 1500 ms 参考上限分四档：`0-375` 使用 success，`376-750` 使用 info，
  `751-1125` 使用 warning，`>1125` 使用 destructive；数值文本始终保留，不能只靠颜色。
- `provider_called` 独立显示是否实际发起云端 HTTP，不能从 `source=provider` 推断。
- 风控记录详情入口在正常记录中显示总 Token；错误记录使用 warning 文字，优先显示
  `error_code`，缺失时回退到“错误”，不能显示无意义的 `0 Token`。详情弹窗复用现有
  Dialog 的分区信息布局，检测内容仅在服务端实际保存脱敏预览时显示。

### 空状态与错误状态

- 空状态复用 `Empty`：图标、标题、说明；只在有真实下一步时提供操作。
- 错误状态复用 `ErrorState`，展示可理解的说明和重试按钮。

## 6. 动效与交互

- 沿用现有全局交互：按钮 active 缩放、桌面卡片轻微 hover、表格行 120ms 状态变化。
- Risk Center 信息卡使用 `disableHoverEffect`；记录列表不新增入场动画、图表或装饰动效。
- 加载使用既有 skeleton shimmer；`prefers-reduced-motion` 下停止动画。
- 每个交互元素必须有可见 focus、disabled 和 active 状态。

## 7. 深度与表面

采用现有“边框 + 轻微 tonal shift”的混合策略：页面背景、卡片背景和 muted 背景形成
层级，卡片以 `ring-1` / `border` 分隔。仅使用现有全局 hover 阴影；Risk Center 的
运维卡关闭 hover elevation，不新增玻璃、发光或更强阴影。

## 8. 可访问性约束与已接受债务

### 约束

- 目标为 WCAG 2.1 AA：正文 4.5:1，大字与非文本控件 3:1。
- 页面必须可用键盘完成刷新、重试和分页；焦点顺序与视觉顺序一致。
- 200% 缩放和 375px 宽度下主内容无横向滚动，长请求 ID/供应商名/类别不破坏布局。
- 状态结果必须有文本；图标为装饰时 `aria-hidden`，分页按钮有明确可翻译名称。
- 主题的 `prefers-reduced-motion` 规则继续生效。

### 已接受债务 / 范围记录

| 项目                                          | 位置                      | 原因                                                                                              | 退出条件                                 |
| --------------------------------------------- | ------------------------- | ------------------------------------------------------------------------------------------------- | ---------------------------------------- |
| 不安装 react-grab / react-scan / react-doctor | `web/default`             | 用户明确拒绝过度设计和依赖/package 变更；本任务用现有测试、tsgo、lint、build 和真实浏览器 QA 覆盖 | 仅在仓库负责人另行批准开发依赖变更时重评 |
| 不新增独立 primitive showcase                 | 共享组件与 `/risk-center` | 现有共享组件、既有 Risk Center 页面和已批准响应式截图已经覆盖本任务使用的 primitive/states        | 只有出现新的 token 或新 primitive 时补充 |
