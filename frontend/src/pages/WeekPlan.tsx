import { useEffect, useMemo, useState } from "react"
import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { dishesApi, weekPlanApi } from "@/api"
import type { DayOverride, Dish, DishIngredient, MealQuota, PlanProfile, WeekPlan as WeekPlanType, WeekPlanPeriodPreferences, WeekPlanPreferences } from "@/types"
import { asArray } from "@/lib/utils"
import { exportWeekPlanAsPng } from "@/lib/weekPlanExport"
import {
  DAY_FULL_LABELS,
  DAY_KEYS,
  MEAL_COUNT_PRESETS,
  PROTEIN_OPTIONS,
  WEEKDAY_PACKS,
  WEEKEND_PACKS,
  applyPacks,
} from "@/lib/weekPlanCombos"
import DayThemeSheet from "@/components/weekplan/DayThemeSheet"
import DayThemeStrip from "@/components/weekplan/DayThemeStrip"
import ComboSheet from "@/components/weekplan/ComboSheet"
import DishImage from "@/components/DishImage"
import PageHeader from "@/components/PageHeader"
import toast from "react-hot-toast"
import {
  CalendarCheck,
  ChevronDown,
  ChefHat,
  Download,
  Flame,
  Heart,
  Leaf,
  Moon,
  Plus,
  RefreshCw,
  Save,
  Search,
  Settings2,
  Soup,
  Sparkles,
  UtensilsCrossed,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react"

type MealType = "lunch" | "dinner"
type PeriodKey = "weekday" | "weekend"
type QuotaKind = "meat_count" | "veg_count" | "soup_count"

const maxKindCount = 5
const mealOrder: MealType[] = ["lunch", "dinner"]

const defaultPrefs: WeekPlanPreferences = {
  weekday: {
    profile: "balanced",
    lunch: { meat_count: 1, veg_count: 1, soup_count: 0 },
    dinner: { meat_count: 1, veg_count: 0, soup_count: 1 },
  },
  weekend: {
    profile: "balanced",
    lunch: { meat_count: 1, veg_count: 1, soup_count: 1 },
    dinner: { meat_count: 1, veg_count: 1, soup_count: 1 },
  },
}

const profileOptions: Array<{
  key: PlanProfile
  label: string
  hint: string
  Icon: LucideIcon
}> = [
  { key: "balanced", label: "均衡", hint: "常规搭配", Icon: Sparkles },
  { key: "quick", label: "快手", hint: "省时间", Icon: Zap },
  { key: "light", label: "清淡", hint: "少负担", Icon: Leaf },
  { key: "spicy", label: "想吃辣", hint: "重口味", Icon: Flame },
  { key: "favorite", label: "收藏", hint: "优先常吃", Icon: Heart },
  { key: "soup", label: "靓汤", hint: "保证有汤", Icon: Soup },
]

const periodMeta: Record<PeriodKey, { label: string; helper: string; tone: string }> = {
  weekday: { label: "工作日", helper: "周一到周五", tone: "primary" },
  weekend: { label: "周末", helper: "周六和周日", tone: "mint" },
}

const mealMeta: Record<MealType, {
  label: string
  shortLabel: string
  soft: string
  text: string
  Icon: LucideIcon
}> = {
  lunch: {
    label: "午餐",
    shortLabel: "午",
    soft: "bg-primary-light",
    text: "text-primary",
    Icon: ChefHat,
  },
  dinner: {
    label: "晚餐",
    shortLabel: "晚",
    soft: "bg-mint-light",
    text: "text-mint",
    Icon: Moon,
  },
}

function normalizePlan(plan?: WeekPlanType): WeekPlanType {
  return {
    warnings: plan?.warnings || [],
    days: (plan?.days || []).map((day) => ({
      ...day,
      lunch: day.lunch || [],
      dinner: day.dinner || [],
    })),
  }
}

function normalizeQuota(quota?: MealQuota): MealQuota {
  return {
    meat_count: clampCount(quota?.meat_count ?? 0),
    veg_count: clampCount(quota?.veg_count ?? 0),
    soup_count: clampCount(quota?.soup_count ?? 0),
  }
}

function normalizePrefs(prefs?: WeekPlanPreferences): WeekPlanPreferences {
  const source = prefs || defaultPrefs
  return {
    weekday: normalizePeriod(source.weekday || defaultPrefs.weekday),
    weekend: normalizePeriod(source.weekend || defaultPrefs.weekend),
    week_want: normalizeWantList(source.week_want),
    days: normalizeDayOverrides(source.days),
  }
}

function normalizeWantList(want?: string[]): string[] | undefined {
  if (!want?.length) return undefined
  const valid = new Set(PROTEIN_OPTIONS.map((option) => option.key))
  const result = want.filter((key, index) => valid.has(key) && want.indexOf(key) === index)
  return result.length ? result : undefined
}

function normalizeDayOverrides(days?: Record<string, DayOverride>): Record<string, DayOverride> | undefined {
  if (!days) return undefined
  const result: Record<string, DayOverride> = {}
  DAY_KEYS.forEach((dayKey) => {
    const override = days[dayKey]
    if (!override) return
    const profile = override.profile && profileOptions.some((option) => option.key === override.profile)
      ? override.profile
      : ""
    const want = normalizeWantList(override.want)
    if (!profile && !want) return
    result[dayKey] = {
      ...(profile ? { profile } : {}),
      ...(want ? { want } : {}),
    }
  })
  return Object.keys(result).length ? result : undefined
}

function normalizePeriod(period: WeekPlanPeriodPreferences): WeekPlanPeriodPreferences {
  return {
    profile: profileOptions.some((option) => option.key === period.profile) ? period.profile : "balanced",
    lunch: normalizeQuota(period.lunch),
    dinner: normalizeQuota(period.dinner),
  }
}

function clampCount(value: number) {
  if (!Number.isFinite(value)) return 0
  return Math.max(0, Math.min(maxKindCount, Math.floor(value)))
}

function quotaTotal(quota: MealQuota) {
  return quota.meat_count + quota.veg_count + quota.soup_count
}

function dishCount(plan: WeekPlanType) {
  return plan.days.reduce((sum, day) => sum + day.lunch.length + day.dinner.length, 0)
}

function skippedMealCount(plan: WeekPlanType) {
  return plan.days.reduce((sum, day) => sum + (day.lunch.length === 0 ? 1 : 0) + (day.dinner.length === 0 ? 1 : 0), 0)
}

function dateRange(plan: WeekPlanType) {
  const first = plan.days[0]?.date
  const last = plan.days[plan.days.length - 1]?.date
  if (!first || !last) return "暂无日期"
  return `${first.slice(5)} - ${last.slice(5)}`
}

function profileLabel(profile: PlanProfile) {
  return profileOptions.find((option) => option.key === profile)?.label || "均衡"
}

function quotaSummary(quota: MealQuota) {
  if (quotaTotal(quota) === 0) return "跳过"
  return `${quota.meat_count}荤${quota.veg_count}素${quota.soup_count}汤`
}

function periodSummary(period: PeriodKey, value: WeekPlanPeriodPreferences) {
  return `${periodMeta[period].label} ${profileLabel(value.profile)} · 午${quotaSummary(value.lunch)} · 晚${quotaSummary(value.dinner)}`
}

function uniqueById(dishes: Dish[]) {
  const seen = new Set<number>()
  return dishes.filter((dish) => {
    if (seen.has(dish.id)) return false
    seen.add(dish.id)
    return true
  })
}

function matchesMeal(dish: Dish, meal: MealType) {
  return !dish.meal_type || dish.meal_type === "all" || dish.meal_type === meal
}

function ingredientName(item: string | DishIngredient) {
  return typeof item === "string" ? item : item.name
}

function dishSearchText(dish: Dish) {
  const ingredients = asArray<string | DishIngredient>(dish.ingredients).map(ingredientName).join(" ")
  const seasonings = asArray<string | DishIngredient>(dish.seasonings).map(ingredientName).join(" ")
  const tags = asArray<string>(dish.tags).join(" ")
  return [dish.name, dish.category, dish.taste, ingredients, seasonings, tags].join(" ").toLowerCase()
}

function CountStepper({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  return (
    <div className="grid h-8 w-[84px] grid-cols-3 overflow-hidden rounded-full border border-border bg-bg shadow-[inset_0_1px_2px_rgba(26,26,46,.04)]">
      <button
        onClick={() => onChange(value - 1)}
        disabled={value <= 0}
        className="flex items-center justify-center text-lg font-extrabold leading-none text-text3 transition-all hover:bg-white hover:text-primary disabled:opacity-30"
        aria-label="减少数量"
        title="减少"
      >
        -
      </button>
      <div className="flex items-center justify-center border-x border-border/70 text-sm font-extrabold text-text">{value}</div>
      <button
        onClick={() => onChange(value + 1)}
        disabled={value >= maxKindCount}
        className="flex items-center justify-center text-lg font-extrabold leading-none text-text3 transition-all hover:bg-white hover:text-primary disabled:opacity-30"
        aria-label="增加数量"
        title="增加"
      >
        +
      </button>
    </div>
  )
}

function ProfileButton({
  option,
  active,
  onClick,
}: {
  option: (typeof profileOptions)[number]
  active: boolean
  onClick: () => void
}) {
  const Icon = option.Icon
  return (
    <button
      onClick={onClick}
      className={`flex min-w-[88px] shrink-0 items-center gap-2 rounded-2xl border px-3 py-2 text-left transition-all active:scale-95 ${
        active ? "border-primary bg-primary-light text-primary" : "border-border bg-card text-text2 hover:border-primary/30"
      }`}
    >
      <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-xl ${active ? "bg-white text-primary" : "bg-bg text-text3"}`}>
        <Icon size={17} strokeWidth={2.35} />
      </span>
      <span className="min-w-0">
        <span className="block text-[13px] font-bold leading-tight">{option.label}</span>
        <span className="block truncate text-[10px] font-medium leading-tight opacity-70">{option.hint}</span>
      </span>
    </button>
  )
}

function MealQuotaEditor({
  meal,
  quota,
  onChange,
}: {
  meal: MealType
  quota: MealQuota
  onChange: (kind: QuotaKind, value: number) => void
}) {
  const meta = mealMeta[meal]
  const Icon = meta.Icon
  const total = quotaTotal(quota)
  return (
    <div className="rounded-2xl border border-border bg-bg/65 p-3">
      <div className="mb-3 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className={`flex h-8 w-8 shrink-0 items-center justify-center rounded-xl ${meta.soft} ${meta.text}`}>
            <Icon size={17} strokeWidth={2.35} />
          </span>
          <div className="min-w-0">
            <div className="text-sm font-extrabold text-text">{meta.label}</div>
            <div className={`text-[11px] font-semibold ${total === 0 ? "text-text3" : meta.text}`}>
              {total === 0 ? "不推荐" : `${total} 道 · ${quota.meat_count}荤 ${quota.veg_count}素 ${quota.soup_count}汤`}
            </div>
          </div>
        </div>
      </div>
      <div className="grid grid-cols-3 gap-2">
        <div className="flex flex-col items-center gap-1.5 rounded-xl bg-card px-2 py-2">
          <span className="text-[12px] font-bold text-text2">荤菜</span>
          <CountStepper value={quota.meat_count} onChange={(value) => onChange("meat_count", value)} />
        </div>
        <div className="flex flex-col items-center gap-1.5 rounded-xl bg-card px-2 py-2">
          <span className="text-[12px] font-bold text-text2">素菜</span>
          <CountStepper value={quota.veg_count} onChange={(value) => onChange("veg_count", value)} />
        </div>
        <div className="flex flex-col items-center gap-1.5 rounded-xl bg-card px-2 py-2">
          <span className="text-[12px] font-bold text-text2">汤</span>
          <CountStepper value={quota.soup_count} onChange={(value) => onChange("soup_count", value)} />
        </div>
      </div>
    </div>
  )
}

function PreferenceCard({
  period,
  value,
  onProfileChange,
  onQuotaChange,
}: {
  period: PeriodKey
  value: WeekPlanPeriodPreferences
  onProfileChange: (profile: PlanProfile) => void
  onQuotaChange: (meal: MealType, kind: QuotaKind, next: number) => void
}) {
  const meta = periodMeta[period]
  return (
    <section className="rounded-[24px] border border-border bg-card p-4 shadow-[0_1px_3px_rgba(0,0,0,.035),0_8px_24px_rgba(26,26,46,.05)]">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="text-[17px] font-extrabold text-text">{meta.label}</div>
          <div className="mt-0.5 text-[12px] font-medium text-text3">{meta.helper}</div>
        </div>
        <span className={`shrink-0 rounded-full px-3 py-1 text-[11px] font-extrabold ${meta.tone === "primary" ? "bg-primary-light text-primary" : "bg-mint-light text-mint"}`}>
          {quotaTotal(value.lunch) + quotaTotal(value.dinner)} 道/天
        </span>
      </div>
      <div className="mb-3 flex gap-2 overflow-x-auto pb-1 scrollbar-none">
        {profileOptions.map((option) => (
          <ProfileButton
            key={option.key}
            option={option}
            active={value.profile === option.key}
            onClick={() => onProfileChange(option.key)}
          />
        ))}
      </div>
      <div className="grid gap-2">
        {mealOrder.map((meal) => (
          <MealQuotaEditor
            key={meal}
            meal={meal}
            quota={value[meal]}
            onChange={(kind, next) => onQuotaChange(meal, kind, next)}
          />
        ))}
      </div>
    </section>
  )
}

function DishChip({
  dish,
  onOpen,
  onRemove,
}: {
  dish: Dish
  onOpen: () => void
  onRemove: () => void
}) {
  return (
    <div className="flex min-w-0 items-center gap-1.5 rounded-full bg-card px-2 py-1 shadow-[inset_0_0_0_1px_rgba(26,26,46,.06)]">
      <button onClick={onOpen} className="min-w-0 truncate text-[12px] font-bold text-text2 transition-colors hover:text-primary">
        {dish.name}
      </button>
      <button
        onClick={onRemove}
        className="flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-text3 transition-all hover:bg-red-light hover:text-red active:scale-90"
        aria-label={`移除${dish.name}`}
        title="移除"
      >
        <X size={12} strokeWidth={2.5} />
      </button>
    </div>
  )
}

function MealSection({
  meal,
  dishes,
  onAdd,
  onOpen,
  onRemove,
}: {
  meal: MealType
  dishes: Dish[]
  onAdd: () => void
  onOpen: (dish: Dish) => void
  onRemove: (dishId: number) => void
}) {
  const meta = mealMeta[meal]
  const Icon = meta.Icon
  return (
    <div className="rounded-2xl bg-bg/70 p-3">
      <div className="mb-2 flex items-center justify-between gap-2">
        <div className="flex min-w-0 items-center gap-2">
          <span className={`flex h-7 w-7 shrink-0 items-center justify-center rounded-xl ${meta.soft} ${meta.text}`}>
            <Icon size={15} strokeWidth={2.35} />
          </span>
          <span className="text-[13px] font-extrabold text-text">{meta.label}</span>
          <span className="text-[11px] font-semibold text-text3">{dishes.length} 道</span>
        </div>
        <button
          onClick={onAdd}
          className="flex h-8 w-8 shrink-0 items-center justify-center rounded-full bg-card text-text3 transition-all hover:bg-primary-light hover:text-primary active:scale-95"
          aria-label={`添加${meta.label}`}
          title="添加"
        >
          <Plus size={15} strokeWidth={2.6} />
        </button>
      </div>
      <div className="flex flex-wrap gap-1.5">
        {dishes.map((dish) => (
          <DishChip key={dish.id} dish={dish} onOpen={() => onOpen(dish)} onRemove={() => onRemove(dish.id)} />
        ))}
      </div>
    </div>
  )
}

function WeekDayCard({
  day,
  onAdd,
  onOpenDish,
  onRemoveDish,
}: {
  day: WeekPlanType["days"][number]
  onAdd: (meal: MealType) => void
  onOpenDish: (dish: Dish) => void
  onRemoveDish: (meal: MealType, dishId: number) => void
}) {
  const visibleMeals = mealOrder.filter((meal) => day[meal].length > 0)

  return (
    <section className="rounded-[24px] border border-border bg-card p-4 shadow-[0_1px_3px_rgba(0,0,0,.035),0_8px_24px_rgba(26,26,46,.05)]">
      <div className="mb-3 flex items-center justify-between gap-3">
        <div className="min-w-0">
          <div className="text-[17px] font-extrabold text-text">{day.day_name}</div>
          <div className="mt-0.5 text-[12px] font-medium text-text3">{day.date}</div>
        </div>
        <span className="shrink-0 rounded-full bg-bg px-3 py-1 text-[11px] font-extrabold text-text3">
          {day.lunch.length + day.dinner.length} 道
        </span>
      </div>
      {visibleMeals.length > 0 && (
        <div className="grid gap-2">
          {visibleMeals.map((meal) => (
            <MealSection
              key={meal}
              meal={meal}
              dishes={day[meal]}
              onAdd={() => onAdd(meal)}
              onOpen={onOpenDish}
              onRemove={(dishId) => onRemoveDish(meal, dishId)}
            />
          ))}
        </div>
      )}
    </section>
  )
}

function DishPickerModal({
  meal,
  selectedIds,
  onAdd,
  onClose,
}: {
  meal: MealType
  selectedIds: Set<number>
  onAdd: (dish: Dish) => void
  onClose: () => void
}) {
  const [search, setSearch] = useState("")
  const [category, setCategory] = useState("全部")
  const meta = mealMeta[meal]
  const { data, isLoading } = useQuery({
    queryKey: ["dishes", "week-plan-picker", meal],
    queryFn: () => dishesApi.list({ enabled: "true", meal_type: meal, pageSize: "100", sort: "sort_order", order: "asc" }),
  })

  const candidates = useMemo(() => (data?.items || []).filter((dish) => matchesMeal(dish, meal)), [data, meal])
  const categories = useMemo(() => {
    const set = new Set<string>()
    candidates.forEach((dish) => {
      if (dish.category) set.add(dish.category)
    })
    return ["全部", ...Array.from(set)]
  }, [candidates])
  const filtered = useMemo(() => {
    const q = search.trim().toLowerCase()
    return candidates.filter((dish) => {
      if (category !== "全部" && dish.category !== category) return false
      if (q && !dishSearchText(dish).includes(q)) return false
      return true
    })
  }, [candidates, category, search])

  return (
    <div className="fixed inset-0 z-[220] flex items-end justify-center" onClick={onClose}>
      <div className="absolute inset-0 bg-black/42" />
      <div
        onClick={(event) => event.stopPropagation()}
        className="relative flex max-h-[86vh] w-full max-w-[640px] flex-col rounded-t-[28px] bg-card shadow-[0_-8px_30px_rgba(0,0,0,.16)] animate-fadeUp"
      >
        <div className="shrink-0 px-5 pt-3">
          <div className="mx-auto mb-4 h-1 w-11 rounded-full bg-border2" />
          <div className="mb-4 flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate text-lg font-extrabold">添加{meta.label}</div>
              <div className="mt-0.5 text-[12px] font-medium text-text3">{filtered.length} 道可选</div>
            </div>
            <button
              onClick={onClose}
              className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-bg text-text3 transition-all hover:text-primary active:scale-95"
              aria-label="关闭"
              title="关闭"
            >
              <X size={18} strokeWidth={2.35} />
            </button>
          </div>

          <label className="relative mb-3 block">
            <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 text-text3" size={17} strokeWidth={2.35} />
            <input
              value={search}
              onChange={(event) => setSearch(event.target.value)}
              placeholder="搜索菜名、分类、配料"
              className="h-11 w-full rounded-2xl border-[1.5px] border-border bg-bg pl-10 pr-4 text-sm font-medium outline-none transition-all focus:border-primary focus:shadow-[0_0_0_3px_rgba(232,115,74,.12)]"
              autoFocus
            />
          </label>

          <div className="flex gap-2 overflow-x-auto pb-3 scrollbar-none">
            {categories.map((item) => (
              <button
                key={item}
                onClick={() => setCategory(item)}
                className={`shrink-0 rounded-full border px-3.5 py-1.5 text-[12px] font-bold transition-all active:scale-95 ${
                  category === item ? "border-primary bg-primary text-white" : "border-border bg-bg text-text2"
                }`}
              >
                {item}
              </button>
            ))}
          </div>
        </div>

        <div className="min-h-0 flex-1 overflow-y-auto px-5 pb-6">
          {isLoading ? (
            <div className="space-y-2 py-3">
              {Array.from({ length: 6 }).map((_, index) => (
                <div key={index} className="h-16 rounded-2xl skeleton" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <div className="py-14 text-center">
              <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-bg text-text3">
                <Search size={23} strokeWidth={2.35} />
              </div>
              <div className="text-sm font-bold text-text2">没有找到合适的菜</div>
            </div>
          ) : (
            <div className="divide-y divide-border/70">
              {filtered.map((dish) => {
                const selected = selectedIds.has(dish.id)
                return (
                  <div key={dish.id} className="flex items-center gap-3 py-3">
                    <div className="h-14 w-14 shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-light to-pink-light">
                      <DishImage dish={dish} className="h-full w-full" emojiSize="text-[27px]" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-extrabold">{dish.name}</div>
                      <div className="mt-1 flex min-w-0 items-center gap-1.5 text-[11px] font-medium text-text3">
                        {dish.category && <span className="max-w-[96px] truncate">{dish.category}</span>}
                        {dish.category && <span className="h-[3px] w-[3px] rounded-full bg-text4" />}
                        <span>{dish.cook_time > 0 ? `${dish.cook_time}分钟` : "时间未填"}</span>
                      </div>
                    </div>
                    <button
                      onClick={() => onAdd(dish)}
                      disabled={selected}
                      className={`flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-full px-3 text-[12px] font-extrabold transition-all active:scale-95 disabled:opacity-55 ${
                        selected ? "bg-bg text-text3" : "bg-primary text-white"
                      }`}
                    >
                      {selected ? "已加入" : <><Plus size={13} strokeWidth={2.7} />加入</>}
                    </button>
                  </div>
                )
              })}
            </div>
          )}
        </div>
      </div>
    </div>
  )
}

function LoadingState() {
  return (
    <div className="px-5 py-4">
      <div className="mx-auto max-w-[640px] space-y-4">
        <div className="h-[220px] rounded-[24px] skeleton" />
        <div className="h-[180px] rounded-[24px] skeleton" />
        <div className="h-[180px] rounded-[24px] skeleton" />
      </div>
    </div>
  )
}

export default function WeekPlan() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const [draftPlan, setDraftPlan] = useState<WeekPlanType>({ days: [] })
  const [draftPrefs, setDraftPrefs] = useState<WeekPlanPreferences>(defaultPrefs)
  const [dirtyPlan, setDirtyPlan] = useState(false)
  const [dirtyPrefs, setDirtyPrefs] = useState(false)
  const [settingsOpen, setSettingsOpen] = useState(false)
  const [picker, setPicker] = useState<{ date: string; meal: MealType } | null>(null)
  const [themeSheet, setThemeSheet] = useState<string | null>(null)
  const [comboOpen, setComboOpen] = useState(false)

  const { data: serverPlan, isLoading: planLoading } = useQuery({
    queryKey: ["week-plan"],
    queryFn: () => weekPlanApi.get(),
  })
  const { data: serverPrefs, isLoading: prefsLoading } = useQuery({
    queryKey: ["week-plan", "preferences"],
    queryFn: () => weekPlanApi.preferences(),
  })

  useEffect(() => {
    if (!serverPlan || dirtyPlan) return
    let cancelled = false
    queueMicrotask(() => {
      if (!cancelled) setDraftPlan(normalizePlan(serverPlan))
    })
    return () => {
      cancelled = true
    }
  }, [dirtyPlan, serverPlan])

  useEffect(() => {
    if (!serverPrefs || dirtyPrefs) return
    let cancelled = false
    queueMicrotask(() => {
      if (!cancelled) setDraftPrefs(normalizePrefs(serverPrefs))
    })
    return () => {
      cancelled = true
    }
  }, [dirtyPrefs, serverPrefs])

  const activePickerDay = picker ? draftPlan.days.find((day) => day.date === picker.date) : null
  const selectedIds = useMemo(() => {
    const set = new Set<number>()
    if (!activePickerDay) return set
    activePickerDay.lunch.forEach((dish) => set.add(dish.id))
    activePickerDay.dinner.forEach((dish) => set.add(dish.id))
    return set
  }, [activePickerDay])

  const regenerateMut = useMutation({
    mutationFn: () => weekPlanApi.regenerate(),
    onSuccess: (plan) => {
      const next = normalizePlan(plan)
      setDraftPlan(next)
      setDirtyPlan(false)
      qc.setQueryData(["week-plan"], next)
      toast.success("已重新生成")
    },
    onError: () => toast.error("重新生成失败"),
  })

  const savePlanMut = useMutation({
    mutationFn: () => weekPlanApi.save(draftPlan),
    onSuccess: (plan) => {
      const next = normalizePlan(plan)
      setDraftPlan(next)
      setDirtyPlan(false)
      qc.setQueryData(["week-plan"], next)
      toast.success("菜单已保存")
    },
    onError: () => toast.error("保存菜单失败"),
  })

  const applyPrefsMut = useMutation({
    mutationFn: async () => {
      const prefs = await weekPlanApi.updatePreferences(draftPrefs)
      const plan = await weekPlanApi.regenerate()
      return { prefs, plan }
    },
    onSuccess: ({ prefs, plan }) => {
      const nextPrefs = normalizePrefs(prefs)
      const nextPlan = normalizePlan(plan)
      setDraftPrefs(nextPrefs)
      setDraftPlan(nextPlan)
      setDirtyPrefs(false)
      setDirtyPlan(false)
      qc.setQueryData(["week-plan", "preferences"], nextPrefs)
      qc.setQueryData(["week-plan"], nextPlan)
      toast.success("已应用设置")
    },
    onError: () => toast.error("应用设置失败"),
  })

  const regenerateDayMut = useMutation({
    mutationFn: async (date: string) => {
      if (dirtyPrefs) {
        const prefs = await weekPlanApi.updatePreferences(draftPrefs)
        const nextPrefs = normalizePrefs(prefs)
        setDraftPrefs(nextPrefs)
        setDirtyPrefs(false)
        qc.setQueryData(["week-plan", "preferences"], nextPrefs)
      }
      return weekPlanApi.regenerateDay(date)
    },
    onSuccess: (plan) => {
      const next = normalizePlan(plan)
      setDraftPlan(next)
      setDirtyPlan(false)
      qc.setQueryData(["week-plan"], next)
      toast.success("这一天已重新生成")
    },
    onError: () => toast.error("重新生成失败"),
  })

  const busy = regenerateMut.isPending || savePlanMut.isPending || applyPrefsMut.isPending || regenerateDayMut.isPending
  const loading = planLoading || prefsLoading

  function updateProfile(period: PeriodKey, profile: PlanProfile) {
    setDraftPrefs((prev) => normalizePrefs({
      ...prev,
      [period]: { ...prev[period], profile },
    }))
    setDirtyPrefs(true)
  }

  function updateQuota(period: PeriodKey, meal: MealType, kind: QuotaKind, value: number) {
    setDraftPrefs((prev) => normalizePrefs({
      ...prev,
      [period]: {
        ...prev[period],
        [meal]: { ...prev[period][meal], [kind]: value },
      },
    }))
    setDirtyPrefs(true)
  }

  function updateDayOverride(dayKey: string, next: DayOverride) {
    setDraftPrefs((prev) => normalizePrefs({
      ...prev,
      days: { ...(prev.days || {}), [dayKey]: next },
    }))
    setDirtyPrefs(true)
  }

  function toggleWeekWant(key: string) {
    setDraftPrefs((prev) => {
      const current = prev.week_want || []
      const next = current.includes(key) ? current.filter((item) => item !== key) : [...current, key]
      return normalizePrefs({ ...prev, week_want: next })
    })
    setDirtyPrefs(true)
  }

  function applyMealCountPreset(preset: (typeof MEAL_COUNT_PRESETS)[number]) {
    setDraftPrefs((prev) => normalizePrefs({
      ...prev,
      weekday: { ...prev.weekday, lunch: { ...preset.weekday.lunch }, dinner: { ...preset.weekday.dinner } },
      weekend: { ...prev.weekend, lunch: { ...preset.weekend.lunch }, dinner: { ...preset.weekend.dinner } },
    }))
    setDirtyPrefs(true)
    toast.success(`已填入${preset.label}，点击应用设置生效`)
  }

  function applyCombo(weekdayKey: string | null, weekendKey: string | null) {
    const weekdayPack = WEEKDAY_PACKS.find((pack) => pack.key === weekdayKey) || null
    const weekendPack = WEEKEND_PACKS.find((pack) => pack.key === weekendKey) || null
    setDraftPrefs((prev) => normalizePrefs(applyPacks(prev, weekdayPack, weekendPack)))
    setDirtyPrefs(true)
    setComboOpen(false)
    toast.success("已套用组合，点击应用设置生效")
  }

  function regenerateThemeDay(dayKey: string) {
    const index = DAY_KEYS.indexOf(dayKey)
    const date = draftPlan.days[index]?.date
    if (!date) {
      toast.error("先生成一周菜单")
      return
    }
    regenerateDayMut.mutate(date)
  }

  function addDish(date: string, meal: MealType, dish: Dish) {
    setDraftPlan((prev) => ({
      warnings: prev.warnings,
      days: prev.days.map((day) => {
        if (day.date !== date) return day
        return { ...day, [meal]: uniqueById([...day[meal], dish]) }
      }),
    }))
    setDirtyPlan(true)
  }

  function removeDish(date: string, meal: MealType, dishId: number) {
    setDraftPlan((prev) => ({
      warnings: prev.warnings,
      days: prev.days.map((day) => {
        if (day.date !== date) return day
        return { ...day, [meal]: day[meal].filter((dish) => dish.id !== dishId) }
      }),
    }))
    setDirtyPlan(true)
  }

  const [exporting, setExporting] = useState(false)

  async function exportPlan() {
    if (draftPlan.days.length === 0) {
      toast.error("暂无可导出的菜单")
      return
    }
    if (exporting) return
    setExporting(true)
    const toastId = toast.loading("正在生成菜单图片…")
    try {
      const fileName = await exportWeekPlanAsPng(draftPlan)
      if (fileName) toast.success("图片已生成，请在下载中查看", { id: toastId })
      else toast.error("暂无可导出的菜单", { id: toastId })
    } catch {
      toast.error("导出失败，请重试", { id: toastId })
    } finally {
      setExporting(false)
    }
  }

  if (loading && draftPlan.days.length === 0) return <LoadingState />

  return (
    <div className="animate-fadeUp">
      <PageHeader
        title="一周菜单"
        subtitle={dateRange(draftPlan)}
        icon={CalendarCheck}
        actions={
          <div className="flex items-center gap-2">
            <button
              onClick={exportPlan}
              disabled={draftPlan.days.length === 0 || exporting}
              className="flex h-9 w-9 items-center justify-center rounded-full bg-card text-text3 shadow-sm transition-all hover:bg-mint-light hover:text-mint active:scale-95 disabled:opacity-45"
              aria-label="导出菜单"
              title={exporting ? "生成中…" : "导出"}
            >
              <Download size={17} strokeWidth={2.35} />
            </button>
            <button
              onClick={() => navigate("/admin/settings#advanced-rules")}
              className="flex h-9 w-9 items-center justify-center rounded-full bg-card text-text3 shadow-sm transition-all hover:bg-primary-light hover:text-primary active:scale-95"
              aria-label="打开高级推荐规则设置"
              title="高级推荐规则"
            >
              <Settings2 size={17} strokeWidth={2.35} />
            </button>
            <button
              onClick={() => savePlanMut.mutate()}
              disabled={busy || !dirtyPlan}
              className="flex h-9 w-9 items-center justify-center rounded-full bg-primary text-white shadow-sm transition-all active:scale-95 disabled:opacity-45"
              aria-label="保存菜单"
              title="保存"
            >
              <Save size={17} strokeWidth={2.35} />
            </button>
          </div>
        }
      />

      <div className="mx-auto max-w-[640px] px-5 py-4">
        <section className="mb-4 rounded-[24px] border border-border bg-card p-4 shadow-[0_1px_3px_rgba(0,0,0,.035),0_8px_24px_rgba(26,26,46,.05)]">
          <div className="grid grid-cols-3 gap-2">
            <div className="rounded-2xl bg-primary-light px-3 py-3">
              <div className="text-[11px] font-bold text-primary/80">总菜数</div>
              <div className="mt-1 text-xl font-extrabold text-primary">{dishCount(draftPlan)}</div>
            </div>
            <div className="rounded-2xl bg-mint-light px-3 py-3">
              <div className="text-[11px] font-bold text-mint/80">不推荐</div>
              <div className="mt-1 text-xl font-extrabold text-mint">{skippedMealCount(draftPlan)}</div>
            </div>
            <div className="rounded-2xl bg-bg px-3 py-3">
              <div className="text-[11px] font-bold text-text3">状态</div>
              <div className="mt-1 truncate text-[13px] font-extrabold text-text">{dirtyPlan || dirtyPrefs ? "有改动" : "已同步"}</div>
            </div>
          </div>
          <div className="mt-3 grid grid-cols-2 gap-2">
            <button
              onClick={() => setSettingsOpen((value) => !value)}
              className="flex h-11 items-center justify-center gap-2 rounded-2xl bg-primary-light text-sm font-extrabold text-primary transition-all active:scale-95"
            >
              <Settings2 size={17} strokeWidth={2.4} />
              {settingsOpen ? "收起设置" : dirtyPrefs ? "设置待应用" : "设置"}
            </button>
            <button
              onClick={() => regenerateMut.mutate()}
              disabled={busy}
              className="flex h-11 items-center justify-center gap-2 rounded-2xl bg-bg text-sm font-extrabold text-text2 transition-all hover:text-primary active:scale-95 disabled:opacity-45"
            >
              <RefreshCw size={17} strokeWidth={2.4} className={regenerateMut.isPending ? "animate-spin" : ""} />
              重生成
            </button>
          </div>
          {draftPlan.warnings && draftPlan.warnings.length > 0 && (
            <div className="mt-3 rounded-2xl border border-primary/20 bg-primary-light/70 px-3 py-2">
              {draftPlan.warnings.slice(0, 3).map((warning) => (
                <div key={warning} className="text-[11px] font-semibold leading-relaxed text-primary">{warning}</div>
              ))}
              {draftPlan.warnings.length > 3 && <div className="text-[11px] font-semibold text-primary/70">还有 {draftPlan.warnings.length - 3} 条提示</div>}
            </div>
          )}
        </section>

        <section className="mb-4 overflow-hidden rounded-[24px] border border-border bg-card shadow-[0_1px_3px_rgba(0,0,0,.035),0_8px_24px_rgba(26,26,46,.05)]">
          <button
            onClick={() => setSettingsOpen((value) => !value)}
            className="flex w-full items-center gap-3 px-4 py-3.5 text-left transition-all active:bg-bg/70"
            aria-expanded={settingsOpen}
          >
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-primary-light text-primary">
              <Settings2 size={20} strokeWidth={2.4} />
            </span>
            <span className="min-w-0 flex-1">
              <span className="flex min-w-0 items-center gap-2">
                <span className="text-[15px] font-extrabold text-text">推荐设置</span>
                {dirtyPrefs && <span className="shrink-0 rounded-full bg-primary-light px-2 py-0.5 text-[10px] font-extrabold text-primary">待应用</span>}
              </span>
              <span className="mt-1 block truncate text-[11px] font-medium text-text3">
                {periodSummary("weekday", draftPrefs.weekday)} / {periodSummary("weekend", draftPrefs.weekend)}
              </span>
            </span>
            <ChevronDown
              size={18}
              strokeWidth={2.45}
              className={`shrink-0 text-text3 transition-transform ${settingsOpen ? "rotate-180" : ""}`}
            />
          </button>

          {settingsOpen && (
            <div className="grid gap-3 border-t border-border bg-bg/45 p-3 animate-fadeUp">
              <div className="rounded-2xl border border-border bg-card p-3">
                <div className="mb-1 text-[13px] font-extrabold text-text">本周想多吃</div>
                <div className="mb-2 text-[11px] font-medium text-text3">选中的食材整周都会优先安排</div>
                <div className="flex flex-wrap gap-2">
                  {PROTEIN_OPTIONS.map((option) => {
                    const active = (draftPrefs.week_want || []).includes(option.key)
                    return (
                      <button
                        key={option.key}
                        onClick={() => toggleWeekWant(option.key)}
                        className={`rounded-full border px-3.5 py-1.5 text-[12px] font-bold transition-all active:scale-95 ${
                          active ? "border-primary bg-primary text-white" : "border-border bg-bg text-text2"
                        }`}
                      >
                        {option.label}
                      </button>
                    )
                  })}
                </div>
              </div>
              <div className="rounded-2xl border border-border bg-card p-3">
                <div className="mb-2 text-[13px] font-extrabold text-text">每天吃几道（快捷填入）</div>
                <div className="grid grid-cols-3 gap-2">
                  {MEAL_COUNT_PRESETS.map((preset) => (
                    <button
                      key={preset.key}
                      onClick={() => applyMealCountPreset(preset)}
                      className="rounded-2xl border border-border bg-bg px-2 py-2 text-center transition-all hover:border-primary/40 active:scale-95"
                    >
                      <span className="block text-[12px] font-extrabold text-text">{preset.label}</span>
                      <span className="mt-0.5 block text-[10px] font-medium leading-tight text-text3">{preset.desc}</span>
                    </button>
                  ))}
                </div>
              </div>
              {(["weekday", "weekend"] as PeriodKey[]).map((period) => (
                <PreferenceCard
                  key={period}
                  period={period}
                  value={draftPrefs[period]}
                  onProfileChange={(profile) => updateProfile(period, profile)}
                  onQuotaChange={(meal, kind, next) => updateQuota(period, meal, kind, next)}
                />
              ))}
              <button
                onClick={() => applyPrefsMut.mutate()}
                disabled={busy || !dirtyPrefs}
                className="flex h-11 items-center justify-center gap-2 rounded-2xl bg-primary text-sm font-extrabold text-white transition-all active:scale-95 disabled:opacity-45"
              >
                <Settings2 size={17} strokeWidth={2.4} />
                应用设置
              </button>
            </div>
          )}
        </section>

        <DayThemeStrip
          days={draftPlan.days}
          overrides={draftPrefs.days || {}}
          defaults={{ weekday: draftPrefs.weekday.profile, weekend: draftPrefs.weekend.profile }}
          onPick={(dayKey) => setThemeSheet(dayKey)}
          onOpenCombos={() => setComboOpen(true)}
        />

        <div className="mb-3 flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <span className="flex h-9 w-9 shrink-0 items-center justify-center rounded-[14px] bg-bg text-text3">
              <UtensilsCrossed size={19} strokeWidth={2.35} />
            </span>
            <div className="min-w-0">
              <div className="text-lg font-extrabold text-text">菜单草稿</div>
              <div className="truncate text-[12px] font-medium text-text3">手动增删后记得保存菜单</div>
            </div>
          </div>
          <button
            onClick={() => savePlanMut.mutate()}
            disabled={busy || !dirtyPlan}
            className="shrink-0 rounded-full bg-primary-light px-3 py-2 text-[12px] font-extrabold text-primary transition-all active:scale-95 disabled:opacity-45"
          >
            保存
          </button>
        </div>

        <div className="grid gap-3">
          {draftPlan.days.length === 0 ? (
            <section className="rounded-[24px] border border-border bg-card px-5 py-12 text-center">
              <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-bg text-text3">
                <CalendarCheck size={24} strokeWidth={2.35} />
              </div>
              <div className="text-sm font-bold text-text2">暂无菜单</div>
              <button
                onClick={() => regenerateMut.mutate()}
                disabled={busy}
                className="mt-4 rounded-full bg-primary px-5 py-2 text-sm font-extrabold text-white transition-all active:scale-95 disabled:opacity-45"
              >
                生成一周菜单
              </button>
            </section>
          ) : (
            draftPlan.days.map((day) => (
              <WeekDayCard
                key={day.date}
                day={day}
                onAdd={(meal) => setPicker({ date: day.date, meal })}
                onOpenDish={(dish) => navigate(`/dishes/${dish.id}`)}
                onRemoveDish={(meal, dishId) => removeDish(day.date, meal, dishId)}
              />
            ))
          )}
        </div>
      </div>

      {picker && (
        <DishPickerModal
          meal={picker.meal}
          selectedIds={selectedIds}
          onAdd={(dish) => addDish(picker.date, picker.meal, dish)}
          onClose={() => setPicker(null)}
        />
      )}

      {themeSheet && (
        <DayThemeSheet
          dayKey={themeSheet}
          dayLabel={DAY_FULL_LABELS[themeSheet] || themeSheet}
          value={draftPrefs.days?.[themeSheet] || {}}
          busy={regenerateDayMut.isPending}
          canRegenerate={Boolean(draftPlan.days[DAY_KEYS.indexOf(themeSheet)]?.date)}
          onChange={(next) => updateDayOverride(themeSheet, next)}
          onRegenerateDay={() => regenerateThemeDay(themeSheet)}
          onClose={() => setThemeSheet(null)}
        />
      )}

      {comboOpen && (
        <ComboSheet onApply={applyCombo} onClose={() => setComboOpen(false)} />
      )}
    </div>
  )
}
