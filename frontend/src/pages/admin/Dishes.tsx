import { useState, useCallback, useRef, useEffect } from "react"
import { useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { dishesApi, settingsApi } from "@/api"
import { asArray } from "@/lib/utils"
import type { Dish, DishTraitAuditItem, DishTraitAuditReport } from "@/types"
import toast from "react-hot-toast"
import { AlertTriangle, CheckCircle2, ChevronDown, RefreshCw, Search, Stethoscope } from "lucide-react"

const categoryEmojis: Record<string, string> = {
  川菜: "🌶", 粤菜: "🐟", 家常菜: "🍳", 快手菜: "⚡",
  汤品: "🍲", 主食: "🍚", 小食: "🥟", 湘菜: "🔥",
  东北菜: "🥬", 新疆菜: "🐑", 云南菜: "🍄", 贵州菜: "🌶",
}
function getEmoji(d: Dish) { return categoryEmojis[d.category] || "🍽" }
function diffLabel(d: string) { return d === "easy" ? "简单" : d === "medium" ? "中等" : "困难" }

function TraitAuditPanel({
  report,
  loading,
  refreshing,
  onRefresh,
  onOpenDish,
}: {
  report?: DishTraitAuditReport
  loading: boolean
  refreshing: boolean
  onRefresh: () => void
  onOpenDish: (dishId: number) => void
}) {
  const [open, setOpen] = useState(false)
  const issueItems = report?.items || []
  const fieldCounts = report?.field_counts || []
  const clean = !!report && report.issue_count === 0
  const visibleItems = open ? issueItems : issueItems.slice(0, 4)

  return (
    <section className="mx-5 mt-3 overflow-hidden rounded-2xl border border-border bg-card shadow-sm">
      <div className="flex items-center justify-between gap-3 px-4 py-3">
        <div className="flex min-w-0 items-center gap-2.5">
          <span className={`flex h-9 w-9 shrink-0 items-center justify-center rounded-xl ${clean ? "bg-mint-light text-mint" : "bg-primary-light text-primary"}`}>
            {clean ? <CheckCircle2 size={18} strokeWidth={2.45} /> : <Stethoscope size={18} strokeWidth={2.45} />}
          </span>
          <div className="min-w-0">
            <div className="text-sm font-extrabold text-text">画像体检</div>
            <div className="truncate text-[11px] font-semibold text-text3">
              {loading ? "检查中" : clean ? `${report?.total || 0} 道菜已通过` : `${report?.issue_dishes || 0} 道菜需复核 · ${report?.issue_count || 0} 项`}
            </div>
          </div>
        </div>
        <button
          onClick={onRefresh}
          disabled={refreshing}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-bg text-text3 transition-all hover:text-primary disabled:opacity-50"
          aria-label="重新体检"
          title="重新体检"
        >
          <RefreshCw size={15} strokeWidth={2.45} className={refreshing ? "animate-spin" : ""} />
        </button>
      </div>

      {loading ? (
        <div className="grid grid-cols-3 gap-2 px-4 pb-4">
          <div className="h-14 rounded-xl skeleton" />
          <div className="h-14 rounded-xl skeleton" />
          <div className="h-14 rounded-xl skeleton" />
        </div>
      ) : report && (
        <>
          <div className="grid grid-cols-3 gap-2 px-4 pb-3">
            <AuditStat label="总菜数" value={report.total} tone="bg-bg text-text2" />
            <AuditStat label="已通过" value={report.passed} tone="bg-mint-light text-mint" />
            <AuditStat label="可疑项" value={report.issue_count} tone={clean ? "bg-bg text-text3" : "bg-primary-light text-primary"} />
          </div>

          {fieldCounts.length > 0 && (
            <div className="flex gap-1.5 overflow-x-auto px-4 pb-3 scrollbar-none">
              {fieldCounts.map((field) => (
                <span key={field.field} className="shrink-0 rounded-full bg-bg px-2.5 py-1 text-[11px] font-bold text-text2">
                  {field.label} {field.count}
                </span>
              ))}
            </div>
          )}

          {issueItems.length > 0 && (
            <div className="border-t border-border bg-bg/45">
              {visibleItems.map((item) => (
                <TraitAuditRow key={item.dish_id} item={item} onOpenDish={onOpenDish} />
              ))}
              {issueItems.length > 4 && (
                <button
                  onClick={() => setOpen((value) => !value)}
                  className="flex w-full items-center justify-center gap-1.5 px-4 py-2.5 text-[12px] font-extrabold text-primary"
                >
                  {open ? "收起" : `查看全部 ${issueItems.length} 道`}
                  <ChevronDown size={14} strokeWidth={2.4} className={open ? "rotate-180 transition-transform" : "transition-transform"} />
                </button>
              )}
            </div>
          )}
        </>
      )}
    </section>
  )
}

function AuditStat({ label, value, tone }: { label: string; value: number; tone: string }) {
  return (
    <div className={`rounded-xl px-3 py-2 ${tone}`}>
      <div className="text-[10px] font-bold opacity-75">{label}</div>
      <div className="mt-0.5 text-lg font-extrabold leading-tight">{value}</div>
    </div>
  )
}

function TraitAuditRow({ item, onOpenDish }: { item: DishTraitAuditItem; onOpenDish: (dishId: number) => void }) {
  const topIssues = item.issues.slice(0, 2)
  return (
    <button
      onClick={() => onOpenDish(item.dish_id)}
      className="flex w-full items-start gap-2.5 border-b border-border/70 px-4 py-3 text-left last:border-0 transition-all active:bg-primary-light/50"
    >
      <span className="mt-0.5 flex h-7 w-7 shrink-0 items-center justify-center rounded-lg bg-primary-light text-primary">
        <AlertTriangle size={15} strokeWidth={2.45} />
      </span>
      <span className="min-w-0 flex-1">
        <span className="flex min-w-0 items-center gap-1.5">
          <span className="truncate text-sm font-extrabold text-text">{item.dish_name}</span>
          <span className="shrink-0 rounded-full bg-card px-2 py-0.5 text-[10px] font-bold text-text3">{item.issues.length} 项</span>
        </span>
        <span className="mt-1 block space-y-0.5">
          {topIssues.map((issue) => (
            <span key={`${issue.field}-${issue.message}`} className="block truncate text-[11px] font-semibold text-text2">
              {issue.label}: {issue.current} → {issue.expected}
            </span>
          ))}
        </span>
      </span>
    </button>
  )
}

export default function AdminDishes() {
  const navigate = useNavigate()
  const qc = useQueryClient()

  const [search, setSearch] = useState("")
  const [filterCategory, setFilterCategory] = useState("")
  const [filterMealType, setFilterMealType] = useState("")
  const [filterDifficulty, setFilterDifficulty] = useState("")
  const [filterEnabled, setFilterEnabled] = useState<string>("")
  const [page, setPage] = useState(1)
  const pageSize = 30

  const [selected, setSelected] = useState<Set<number>>(new Set())
  const [batchMode, setBatchMode] = useState(false)
  const [showBatchCat, setShowBatchCat] = useState(false)
  const [batchCategory, setBatchCategory] = useState("")
  const [showSearch, setShowSearch] = useState(false)
  const searchRef = useRef<HTMLInputElement>(null)

  useEffect(() => {
    if (showSearch && searchRef.current) searchRef.current.focus()
  }, [showSearch])

  const { data: settings } = useQuery({ queryKey: ["settings"], queryFn: () => settingsApi.get() })
  const categories = asArray<string>(settings?.categories).filter((x) => typeof x === "string")
  if (categories.length === 0) categories.push("家常菜", "川菜", "粤菜", "快手菜", "汤品", "主食")

  const params: Record<string, string> = { pageSize: String(pageSize), page: String(page) }
  if (search) params.search = search
  if (filterCategory) params.category = filterCategory
  if (filterMealType) params.meal_type = filterMealType
  if (filterDifficulty) params.difficulty = filterDifficulty
  if (filterEnabled) params.enabled = filterEnabled

  const { data: dishesData, isLoading } = useQuery({
    queryKey: ["admin", "dishes", params],
    queryFn: () => dishesApi.list(params),
  })
  const { data: traitAudit, isLoading: auditLoading, isFetching: auditFetching, refetch: refetchAudit } = useQuery({
    queryKey: ["admin", "dishes", "trait-audit"],
    queryFn: () => dishesApi.traitAudit(),
  })
  const dishes = dishesData?.items || []
  const total = dishesData?.total || 0
  const totalPages = Math.ceil(total / pageSize)

  const deleteMut = useMutation({
    mutationFn: (id: number) => dishesApi.delete(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "dishes"] }); toast.success("已删除") },
  })

  const cloneMut = useMutation({
    mutationFn: (id: number) => dishesApi.clone(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "dishes"] }); toast.success("已克隆") },
  })

  const toggleMut = useMutation({
    mutationFn: (id: number) => dishesApi.toggle(id),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "dishes"] }) },
  })

  const batchToggleMut = useMutation({
    mutationFn: ({ ids, enabled }: { ids: number[]; enabled: boolean }) => dishesApi.batchToggle(ids, enabled),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "dishes"] }); setSelected(new Set()); toast.success("批量操作成功") },
  })

  const batchDeleteMut = useMutation({
    mutationFn: (ids: number[]) => dishesApi.batchDelete(ids),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "dishes"] }); setSelected(new Set()); toast.success("批量删除成功") },
  })

  const batchCatMut = useMutation({
    mutationFn: ({ ids, category }: { ids: number[]; category: string }) => dishesApi.batchCategory(ids, category),
    onSuccess: () => { qc.invalidateQueries({ queryKey: ["admin", "dishes"] }); setSelected(new Set()); setShowBatchCat(false); toast.success("分类已修改") },
  })

  const toggleSelect = useCallback((id: number) => {
    setSelected((prev) => {
      const next = new Set(prev)
      if (next.has(id)) next.delete(id)
      else next.add(id)
      return next
    })
  }, [])

  const selectAll = useCallback(() => {
    if (selected.size === dishes.length) setSelected(new Set())
    else setSelected(new Set(dishes.map((d: Dish) => d.id)))
  }, [dishes, selected.size])

  const hasFilters = search || filterCategory || filterMealType || filterDifficulty || filterEnabled

  const clearFilters = () => {
    setSearch("")
    setFilterCategory("")
    setFilterMealType("")
    setFilterDifficulty("")
    setFilterEnabled("")
    setPage(1)
  }

  const pillCls = (active: boolean) =>
    `px-3 py-1.5 rounded-full text-xs border-[1.5px] transition-all whitespace-nowrap flex-shrink-0 ${active ? "bg-primary text-white border-primary" : "border-border text-text2"}`

  const miniCls = (active: boolean, variant?: "mint" | "red") => {
    if (active) return "px-2.5 py-1 rounded-full text-[11px] font-semibold bg-primary text-white"
    if (variant === "mint") return "px-2.5 py-1 rounded-full text-[11px] font-semibold border border-border text-text3"
    if (variant === "red") return "px-2.5 py-1 rounded-full text-[11px] font-semibold border border-border text-text3"
    return "px-2.5 py-1 rounded-full text-[11px] font-semibold border border-border text-text3"
  }

  const enabledOpts = [
    { key: "", label: "全部" },
    { key: "true", label: "启用" },
    { key: "false", label: "禁用" },
  ]

  return (
    <div className="py-4 max-w-[640px] mx-auto pb-20">
      <div className="flex items-center justify-between mb-3 px-5">
        <div className="text-lg font-bold">菜品管理</div>
        <div className="flex items-center gap-1.5">
          <button
            onClick={() => setShowSearch(!showSearch)}
            className={`w-8 h-8 rounded-full flex items-center justify-center transition-all ${showSearch || search ? "bg-primary text-white" : "bg-card border border-border text-text2"}`}
          >
            <Search size={15} />
          </button>
          <button
            onClick={() => { setBatchMode(!batchMode); setSelected(new Set()) }}
            className={`px-3 py-1.5 rounded-full text-xs font-semibold border-[1.5px] transition-all ${batchMode ? "bg-primary text-white border-primary" : "border-border text-text2"}`}
          >
            {batchMode ? "取消" : "批量"}
          </button>
          <button onClick={() => navigate("/admin/dishes/new")} className="inline-flex items-center justify-center gap-1.5 py-1.5 px-4 rounded-full text-xs font-semibold bg-primary text-white transition-all active:scale-96">
            + 添加
          </button>
        </div>
      </div>

      {showSearch && (
        <div className="mb-2 px-5">
          <div className="flex items-stretch rounded-xl border-[1.5px] border-border bg-card overflow-hidden transition-all focus-within:border-primary">
            <Search size={15} className="flex-shrink-0 my-auto ml-3 text-text3" />
            <input
              ref={searchRef}
              type="text"
              value={search}
              onChange={(e) => { setSearch(e.target.value); setPage(1) }}
              placeholder="搜索菜品名称..."
              className="flex-1 min-w-0 py-2 px-2.5 bg-transparent text-sm outline-none"
            />
            {search && (
              <button onClick={() => { setSearch(""); setPage(1) }} className="flex-shrink-0 px-2.5 text-text3 text-xs hover:text-primary transition-colors">✕</button>
            )}
          </div>
        </div>
      )}

      <div className="flex gap-1.5 overflow-x-auto overscroll-x-contain scrollbar-none px-5" style={{ WebkitOverflowScrolling: "touch" }}>
        <button onClick={() => { setFilterCategory(""); setPage(1) }} className={pillCls(filterCategory === "")}>全部</button>
        {categories.map((c) => (
          <button key={c} onClick={() => { setFilterCategory(filterCategory === c ? "" : c); setPage(1) }} className={pillCls(filterCategory === c)}>
            {c}
          </button>
        ))}
      </div>

      <div className="flex items-center gap-1 mt-2 px-5 flex-wrap">
        <span className="text-[10px] text-text3 font-semibold mr-0.5">餐次</span>
        <button onClick={() => { setFilterMealType(""); setPage(1) }} className={miniCls(filterMealType === "")}>全部</button>
        <button onClick={() => { setFilterMealType("lunch"); setPage(1) }} className={miniCls(filterMealType === "lunch")}>午餐</button>
        <button onClick={() => { setFilterMealType("dinner"); setPage(1) }} className={miniCls(filterMealType === "dinner")}>晚餐</button>
        <span className="w-px h-4 bg-border mx-1" />
        {enabledOpts.map((opt) => (
          <button
            key={opt.key}
            onClick={() => { setFilterEnabled(opt.key); setPage(1) }}
            className={`px-2.5 py-1 rounded-full text-[11px] font-semibold transition-all ${
              filterEnabled === opt.key
                ? opt.key === "false" ? "bg-red-light text-red"
                  : opt.key === "true" ? "bg-mint-light text-mint"
                  : "bg-primary text-white"
                : "border border-border text-text3"
            }`}
          >
            {opt.label}
          </button>
        ))}
      </div>

      <div className="flex items-center gap-1 mt-1.5 px-5">
        <span className="text-[10px] text-text3 font-semibold mr-0.5">难度</span>
        <button onClick={() => { setFilterDifficulty(""); setPage(1) }} className={miniCls(filterDifficulty === "")}>全部</button>
        <button onClick={() => { setFilterDifficulty("easy"); setPage(1) }} className={miniCls(filterDifficulty === "easy")}>简单</button>
        <button onClick={() => { setFilterDifficulty("medium"); setPage(1) }} className={miniCls(filterDifficulty === "medium")}>中等</button>
        <button onClick={() => { setFilterDifficulty("hard"); setPage(1) }} className={miniCls(filterDifficulty === "hard")}>困难</button>
      </div>

      {hasFilters && (
        <div className="flex items-center gap-2 mt-2 px-5">
          <span className="text-[11px] text-text2">共 {total} 条</span>
          <button onClick={clearFilters} className="text-[11px] text-primary">清除筛选</button>
        </div>
      )}

      <TraitAuditPanel
        report={traitAudit}
        loading={auditLoading}
        refreshing={auditFetching}
        onRefresh={() => refetchAudit()}
        onOpenDish={(dishId) => navigate(`/admin/dishes/${dishId}`)}
      />

      <div className="px-5 mt-3">
        {batchMode && selected.size > 0 && (
          <div className="mb-3 bg-primary-light rounded-2xl p-3 flex items-center gap-2 flex-wrap">
            <span className="text-xs font-semibold text-primary">已选 {selected.size} 项</span>
            <button onClick={selectAll} className="px-2.5 py-1 rounded-full text-[11px] bg-card font-semibold">
              {selected.size === dishes.length ? "取消全选" : "全选当前页"}
            </button>
            <button onClick={() => batchToggleMut.mutate({ ids: [...selected], enabled: true })} className="px-2.5 py-1 rounded-full text-[11px] bg-mint text-white font-semibold">批量启用</button>
            <button onClick={() => batchToggleMut.mutate({ ids: [...selected], enabled: false })} className="px-2.5 py-1 rounded-full text-[11px] bg-text3 text-white font-semibold">批量禁用</button>
            <button onClick={() => setShowBatchCat(true)} className="px-2.5 py-1 rounded-full text-[11px] bg-primary text-white font-semibold">改分类</button>
            <button onClick={() => { if (confirm(`确定删除选中的 ${selected.size} 道菜品？`)) batchDeleteMut.mutate([...selected]) }} className="px-2.5 py-1 rounded-full text-[11px] bg-red-500 text-white font-semibold">批量删除</button>
          </div>
        )}

        {isLoading ? (
          <div className="py-12 text-center text-text2">加载中...</div>
        ) : (
          <div className="bg-card rounded-2xl overflow-hidden shadow-sm border border-border">
            {dishes.map((d: Dish) => (
              <div
                key={d.id}
                onClick={() => { if (!batchMode) navigate(`/admin/dishes/${d.id}`) }}
                className={`flex items-center gap-3 px-4 py-3 border-b border-border last:border-0 transition-all ${!d.enabled ? "opacity-50" : ""} ${batchMode ? "cursor-default" : "active:bg-bg cursor-pointer"} ${selected.has(d.id) ? "bg-primary-light/50" : ""}`}
              >
                {batchMode && (
                  <button
                    onClick={() => toggleSelect(d.id)}
                    className={`w-5 h-5 rounded-md border-2 flex items-center justify-center flex-shrink-0 transition-all ${selected.has(d.id) ? "bg-primary border-primary text-white" : "border-border2"}`}
                  >
                    {selected.has(d.id) && <span className="text-xs">✓</span>}
                  </button>
                )}
                <div className="w-10 h-10 rounded-[10px] bg-primary-light flex items-center justify-center text-xl flex-shrink-0">
                  {d.image_url ? <img src={d.image_url} alt="" className="w-full h-full object-cover rounded-[10px]" /> : getEmoji(d)}
                </div>
                <div className="flex-1 min-w-0 overflow-hidden">
                  <div className="text-sm font-semibold truncate">{d.name}</div>
                  <div className="text-[11px] text-text2 truncate">{d.category} · {d.cook_time}分钟 · {diffLabel(d.difficulty)} · {d.meal_type === "lunch" ? "午餐" : d.meal_type === "dinner" ? "晚餐" : "通用"}</div>
                </div>
                {!batchMode && (
                  <div className="flex gap-1 flex-shrink-0">
                    <button
                      onClick={(e) => { e.stopPropagation(); toggleMut.mutate(d.id) }}
                      className={`w-7 h-7 rounded-lg flex items-center justify-center text-[10px] font-bold flex-shrink-0 transition-all active:scale-95 ${d.enabled ? "bg-mint-light text-mint" : "bg-bg text-text3"}`}
                    >
                      {d.enabled ? "开" : "关"}
                    </button>
                    <button onClick={(e) => { e.stopPropagation(); cloneMut.mutate(d.id) }} className="w-7 h-7 rounded-lg flex items-center justify-center text-xs bg-bg transition-all active:bg-primary-light flex-shrink-0">📋</button>
                    <button onClick={(e) => { e.stopPropagation(); navigate(`/admin/dishes/${d.id}`) }} className="w-7 h-7 rounded-lg flex items-center justify-center text-xs bg-bg transition-all active:bg-primary-light flex-shrink-0">✎</button>
                    <button onClick={(e) => { e.stopPropagation(); if (confirm(`确定删除「${d.name}」吗？`)) deleteMut.mutate(d.id) }} className="w-7 h-7 rounded-lg flex items-center justify-center text-xs bg-bg transition-all active:bg-red-50 active:text-red-500 flex-shrink-0">🗑</button>
                  </div>
                )}
              </div>
            ))}
            {dishes.length === 0 && (
              <div className="py-12 text-center text-sm text-text3">{hasFilters ? "没有匹配的菜品" : "还没有菜品，去添加第一道菜吧"}</div>
            )}
          </div>
        )}

        {totalPages > 1 && (
          <div className="flex items-center justify-center gap-2 mt-4">
            <button onClick={() => setPage(1)} disabled={page <= 1} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">首页</button>
            <button onClick={() => setPage((p) => Math.max(1, p - 1))} disabled={page <= 1} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">上一页</button>
            <span className="text-xs text-text2">{page}/{totalPages}</span>
            <button onClick={() => setPage((p) => Math.min(totalPages, p + 1))} disabled={page >= totalPages} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">下一页</button>
            <button onClick={() => setPage(totalPages)} disabled={page >= totalPages} className="px-3 py-1.5 rounded-full text-xs border border-border disabled:opacity-30">末页</button>
          </div>
        )}
      </div>

      {showBatchCat && (
        <div className="fixed inset-0 z-[300] flex items-center justify-center p-6 bg-black/40 backdrop-blur-sm" onClick={() => setShowBatchCat(false)}>
          <div className="bg-card rounded-2xl p-5 w-full max-w-[320px] shadow-xl" onClick={(e) => e.stopPropagation()}>
            <div className="text-base font-bold mb-3">批量修改分类</div>
            <div className="flex flex-wrap gap-2 mb-4">
              {categories.map((c) => (
                <button key={c} onClick={() => setBatchCategory(c)} className={`px-3 py-1.5 rounded-full text-xs border-[1.5px] transition-all ${batchCategory === c ? "bg-primary text-white border-primary" : "border-border text-text2"}`}>
                  {c}
                </button>
              ))}
            </div>
            <div className="flex gap-2.5">
              <button onClick={() => setShowBatchCat(false)} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold border-[1.5px] border-border2">取消</button>
              <button onClick={() => batchCategory && batchCatMut.mutate({ ids: [...selected], category: batchCategory })} disabled={!batchCategory} className="flex-1 py-2.5 px-5 rounded-full text-sm font-semibold bg-primary text-white disabled:opacity-50">确定</button>
            </div>
          </div>
        </div>
      )}
    </div>
  )
}
