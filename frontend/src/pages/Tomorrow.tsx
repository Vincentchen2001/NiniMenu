import { useEffect, useMemo, useRef, useState } from "react"
import { createPortal } from "react-dom"
import { useNavigate } from "react-router-dom"
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query"
import { dishesApi, pickApi, recordsApi, settingsApi } from "@/api"
import type { Dish, DishIngredient, MealRecord } from "@/types"
import { asArray, asString } from "@/lib/utils"
import { launchConfetti } from "@/lib/confetti"
import DishImage from "@/components/DishImage"
import PageHeader from "@/components/PageHeader"
import toast from "react-hot-toast"
import {
  CalendarClock,
  Check,
  ChefHat,
  Clock,
  Dice5,
  Flame,
  Heart,
  Leaf,
  Minus,
  Moon,
  Plus,
  RefreshCw,
  Search,
  ShoppingBasket,
  Sparkles,
  Trash2,
  UtensilsCrossed,
  X,
  Zap,
  type LucideIcon,
} from "lucide-react"

type MealType = "lunch" | "dinner"
type PlanProfile = "balanced" | "quick" | "light" | "spicy" | "favorite"

const weekdayNames = ["周日", "周一", "周二", "周三", "周四", "周五", "周六"]
const mealOrder: MealType[] = ["lunch", "dinner"]
const fallbackTargets: Record<MealType, number> = { lunch: 2, dinner: 2 }
const maxPerMeal = 5

const profileOptions: Array<{
  key: PlanProfile
  label: string
  hint: string
  Icon: LucideIcon
}> = [
  { key: "balanced", label: "均衡", hint: "正常搭配", Icon: Sparkles },
  { key: "quick", label: "快手", hint: "省时间", Icon: Zap },
  { key: "light", label: "清淡", hint: "少负担", Icon: Leaf },
  { key: "spicy", label: "想吃辣", hint: "重口味", Icon: Flame },
  { key: "favorite", label: "收藏", hint: "优先常吃", Icon: Heart },
]

const mealMeta: Record<MealType, {
  label: string
  shortLabel: string
  helper: string
  accent: string
  soft: string
  text: string
  Icon: LucideIcon
}> = {
  lunch: {
    label: "明天午餐",
    shortLabel: "午餐",
    helper: "中午吃得稳一点",
    accent: "bg-primary",
    soft: "bg-primary-light",
    text: "text-primary",
    Icon: ChefHat,
  },
  dinner: {
    label: "明天晚餐",
    shortLabel: "晚餐",
    helper: "晚上吃得舒服一点",
    accent: "bg-mint",
    soft: "bg-mint-light",
    text: "text-mint",
    Icon: Moon,
  },
}

function getTomorrow() {
  const d = new Date()
  d.setDate(d.getDate() + 1)
  const str = `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
  return { str, label: `${str.slice(5)} ${weekdayNames[d.getDay()]}` }
}

function targetFromSetting(value: unknown, fallback: number) {
  const raw = asString(value, String(fallback)).trim()
  if (raw === "") return fallback
  const parsed = Number(raw)
  if (!Number.isFinite(parsed) || parsed < 0) return fallback
  return Math.min(maxPerMeal, Math.floor(parsed))
}

function diffLabel(difficulty: string) {
  if (difficulty === "easy") return "简单"
  if (difficulty === "medium") return "中等"
  return "费点劲"
}

function difficultyTone(difficulty: string) {
  if (difficulty === "easy") return "bg-mint-light text-mint"
  if (difficulty === "medium") return "bg-yellow-light text-yellow"
  return "bg-primary-light text-primary"
}

function cookTimeText(dish: Dish) {
  return dish.cook_time > 0 ? `${dish.cook_time}分钟` : "时间未填"
}

function totalCookTime(dishes: Dish[]) {
  return dishes.reduce((sum, d) => sum + Math.max(0, d.cook_time || 0), 0)
}

function matchesMeal(dish: Dish, meal: MealType) {
  return !dish.meal_type || dish.meal_type === "all" || dish.meal_type === meal
}

function uniqueById(items: Dish[]) {
  const seen = new Set<number>()
  return items.filter((item) => {
    if (seen.has(item.id)) return false
    seen.add(item.id)
    return true
  })
}

function mealRecords(records: MealRecord[], meal: MealType) {
  return records
    .filter((r) => r.meal_type === meal)
    .slice()
    .sort((a, b) => a.id - b.id)
}

function keyFromRecords(records: MealRecord[]) {
  return mealOrder
    .map((meal) => `${meal}:${mealRecords(records, meal).map((r) => r.dish_id).join(",")}`)
    .join("|")
}

function keyFromDishes(lunch: Dish[], dinner: Dish[]) {
  return `lunch:${lunch.map((d) => d.id).join(",")}|dinner:${dinner.map((d) => d.id).join(",")}`
}

function recordToDish(record: MealRecord): Dish {
  return {
    id: record.dish_id,
    name: record.dish_name,
    image_url: record.dish_image_url || "",
    video_url: "",
    images: [],
    category: "",
    meal_type: record.meal_type,
    taste: "",
    ingredients: [],
    seasonings: [],
    steps: [],
    cook_time: 0,
    difficulty: "easy",
    remark: "",
    favorite: false,
    enabled: true,
    tags: [],
    sort_order: 0,
    created_at: record.created_at,
    updated_at: record.created_at,
  }
}

function itemName(item: string | DishIngredient) {
  return typeof item === "string" ? item : item.name
}

function dishSearchText(dish: Dish) {
  const ingredients = asArray<string | DishIngredient>(dish.ingredients).map(itemName).join(" ")
  const seasonings = asArray<string | DishIngredient>(dish.seasonings).map(itemName).join(" ")
  const tags = asArray<string>(dish.tags).join(" ")
  return [dish.name, dish.category, dish.taste, diffLabel(dish.difficulty), ingredients, seasonings, tags]
    .join(" ")
    .toLowerCase()
}

function CountStepper({ value, onChange }: { value: number; onChange: (value: number) => void }) {
  return (
    <div className="grid h-8 grid-cols-[32px_32px_32px] overflow-hidden rounded-full border border-border bg-bg">
      <button
        onClick={() => onChange(Math.max(0, value - 1))}
        disabled={value <= 0}
        className="flex items-center justify-center text-text3 transition-all hover:text-primary disabled:opacity-30"
        aria-label="减少数量"
        title="减少数量"
      >
        <Minus size={14} strokeWidth={2.4} />
      </button>
      <div className="flex items-center justify-center text-sm font-extrabold text-text">{value}</div>
      <button
        onClick={() => onChange(Math.min(maxPerMeal, value + 1))}
        disabled={value >= maxPerMeal}
        className="flex items-center justify-center text-text3 transition-all hover:text-primary disabled:opacity-30"
        aria-label="增加数量"
        title="增加数量"
      >
        <Plus size={14} strokeWidth={2.4} />
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
      className={`flex min-w-[86px] shrink-0 items-center gap-2 rounded-2xl border px-3 py-2 text-left transition-all active:scale-95 ${
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

function PlanSummary({
  dateLabel,
  totalCount,
  hasSavedPlan,
  hasChanges,
  lunch,
  dinner,
  onGenerateAll,
  onOpenShopping,
  disabled,
}: {
  dateLabel: string
  totalCount: number
  hasSavedPlan: boolean
  hasChanges: boolean
  lunch: Dish[]
  dinner: Dish[]
  onGenerateAll: () => void
  onOpenShopping: () => void
  disabled: boolean
}) {
  const savedText = hasSavedPlan ? (hasChanges ? "有修改" : "已保存") : "待保存"
  const savedTone = hasSavedPlan && !hasChanges ? "bg-mint-light text-mint" : "bg-yellow-light text-yellow"
  const lunchMinutes = totalCookTime(lunch)
  const dinnerMinutes = totalCookTime(dinner)

  return (
    <section className="rounded-[24px] border border-border bg-card p-4 shadow-[0_1px_3px_rgba(0,0,0,.035),0_8px_24px_rgba(26,26,46,.05)]">
      <div className="mb-4 flex items-start justify-between gap-3">
        <div className="min-w-0">
          <div className="mb-1 flex items-center gap-2">
            <span className="flex h-10 w-10 shrink-0 items-center justify-center rounded-[14px] bg-primary-light text-primary">
              <CalendarClock size={22} strokeWidth={2.45} />
            </span>
            <div className="min-w-0">
              <div className="truncate text-[22px] font-extrabold leading-tight tracking-tight">明天菜单</div>
              <div className="truncate text-[12px] font-semibold text-text3">{dateLabel}</div>
            </div>
          </div>
        </div>
        <span className={`shrink-0 rounded-full px-3 py-1 text-[11px] font-extrabold ${savedTone}`}>{savedText}</span>
      </div>

      <div className="mb-4 grid grid-cols-3 gap-2">
        <div className="rounded-2xl bg-bg px-3 py-2.5">
          <div className="text-[11px] font-semibold text-text3">菜品</div>
          <div className="text-lg font-extrabold leading-tight">{totalCount} 道</div>
        </div>
        <div className="rounded-2xl bg-bg px-3 py-2.5">
          <div className="text-lg font-extrabold leading-tight">午餐 {lunch.length} 道</div>
          <div className="mt-0.5 truncate text-[11px] font-semibold text-text3">
            约用时 {lunchMinutes > 0 ? `${lunchMinutes}m` : "-"}
          </div>
        </div>
        <div className="rounded-2xl bg-bg px-3 py-2.5">
          <div className="text-lg font-extrabold leading-tight">晚餐 {dinner.length} 道</div>
          <div className="mt-0.5 truncate text-[11px] font-semibold text-text3">
            约用时 {dinnerMinutes > 0 ? `${dinnerMinutes}m` : "-"}
          </div>
        </div>
      </div>

      <div className="grid grid-cols-2 gap-2.5">
        <button
          onClick={onGenerateAll}
          disabled={disabled}
          className="flex h-11 items-center justify-center gap-2 rounded-2xl bg-primary text-sm font-extrabold text-white transition-all hover:bg-primary-dark active:scale-95 disabled:opacity-50"
        >
          <Dice5 size={17} strokeWidth={2.5} />
          重新生成
        </button>
        <button
          onClick={onOpenShopping}
          className="flex h-11 items-center justify-center gap-2 rounded-2xl border border-border bg-bg text-sm font-extrabold text-text2 transition-all hover:border-mint/40 hover:text-mint active:scale-95"
        >
          <ShoppingBasket size={17} strokeWidth={2.35} />
          买菜清单
        </button>
      </div>
    </section>
  )
}

function DishRow({
  dish,
  meal,
  onOpen,
  onSwap,
  onRemove,
}: {
  dish: Dish
  meal: MealType
  onOpen: () => void
  onSwap: () => void
  onRemove: () => void
}) {
  const meta = mealMeta[meal]
  return (
    <div className="flex items-center gap-3 py-3">
      <button onClick={onOpen} className="h-16 w-16 shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-light to-pink-light" aria-label={`查看${dish.name}`}>
        <DishImage dish={dish} className="h-full w-full" emojiSize="text-[30px]" />
      </button>
      <button onClick={onOpen} className="min-w-0 flex-1 text-left">
        <div className="mb-1 truncate text-[15px] font-extrabold leading-tight">{dish.name}</div>
        <div className="flex min-w-0 flex-wrap items-center gap-1.5">
          {dish.category && <span className="max-w-[92px] truncate rounded-full bg-bg px-2 py-0.5 text-[10px] font-semibold text-text2">{dish.category}</span>}
          <span className="flex items-center gap-1 rounded-full bg-bg px-2 py-0.5 text-[10px] font-semibold text-text2">
            <Clock size={10} strokeWidth={2.5} />
            {cookTimeText(dish)}
          </span>
          <span className={`rounded-full px-2 py-0.5 text-[10px] font-semibold ${difficultyTone(dish.difficulty)}`}>{diffLabel(dish.difficulty)}</span>
        </div>
      </button>
      <div className="flex shrink-0 items-center gap-1">
        <button
          onClick={onSwap}
          className={`flex h-9 w-9 items-center justify-center rounded-full ${meta.soft} ${meta.text} transition-all active:scale-90`}
          aria-label={`替换${dish.name}`}
          title="替换"
        >
          <RefreshCw size={16} strokeWidth={2.35} />
        </button>
        <button
          onClick={onRemove}
          className="flex h-9 w-9 items-center justify-center rounded-full bg-bg text-text3 transition-all hover:bg-red-light hover:text-red active:scale-90"
          aria-label={`移除${dish.name}`}
          title="移除"
        >
          <Trash2 size={16} strokeWidth={2.25} />
        </button>
      </div>
    </div>
  )
}

function MealPlanner({
  meal,
  dishes,
  target,
  disabled,
  onTargetChange,
  onOpenDish,
  onFill,
  onReplaceMeal,
  onSwapDish,
  onRemoveDish,
  onOpenPicker,
}: {
  meal: MealType
  dishes: Dish[]
  target: number
  disabled: boolean
  onTargetChange: (value: number) => void
  onOpenDish: (dish: Dish) => void
  onFill: () => void
  onReplaceMeal: () => void
  onSwapDish: (dishId: number) => void
  onRemoveDish: (dishId: number) => void
  onOpenPicker: () => void
}) {
  const meta = mealMeta[meal]
  const Icon = meta.Icon
  const missing = Math.max(0, target - dishes.length)
  const minutes = totalCookTime(dishes)

  return (
    <section className="overflow-hidden rounded-[24px] border border-border bg-card shadow-[0_1px_3px_rgba(0,0,0,.035),0_8px_24px_rgba(26,26,46,.05)]">
      <div className="border-b border-border px-4 py-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-3">
            <span className={`flex h-11 w-11 shrink-0 items-center justify-center rounded-[15px] ${meta.soft} ${meta.text}`}>
              <Icon size={23} strokeWidth={2.45} />
            </span>
            <div className="min-w-0">
              <div className="truncate text-[17px] font-extrabold leading-tight">{meta.label}</div>
              <div className="mt-0.5 flex min-w-0 items-center gap-1.5 text-[11px] font-semibold text-text3">
                <span className="min-w-0 truncate">{meta.helper}</span>
                {minutes > 0 && (
                  <>
                    <span className="h-[3px] w-[3px] shrink-0 rounded-full bg-text4" />
                    <span className="shrink-0">约 {minutes} 分钟</span>
                  </>
                )}
              </div>
            </div>
          </div>
          <CountStepper value={target} onChange={onTargetChange} />
        </div>
      </div>

      <div className="px-4">
        {dishes.length === 0 ? (
          <div className="py-7 text-center">
            <div className={`mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl ${meta.soft} ${meta.text}`}>
              <UtensilsCrossed size={24} strokeWidth={2.35} />
            </div>
            <div className="text-sm font-bold text-text">还没选{meta.shortLabel}</div>
            <div className="mt-1 text-[12px] text-text3">可以智能补齐，也可以自己挑</div>
          </div>
        ) : (
          <div className="divide-y divide-border/70">
            {dishes.map((dish) => (
              <DishRow
                key={dish.id}
                dish={dish}
                meal={meal}
                onOpen={() => onOpenDish(dish)}
                onSwap={() => onSwapDish(dish.id)}
                onRemove={() => onRemoveDish(dish.id)}
              />
            ))}
          </div>
        )}
      </div>

      <div className="grid grid-cols-3 gap-2 border-t border-border bg-bg/55 px-4 py-3">
        <button
          onClick={onFill}
          disabled={disabled || missing === 0}
          className="flex h-10 items-center justify-center gap-1.5 rounded-2xl bg-card text-[12px] font-extrabold text-text2 transition-all hover:text-primary active:scale-95 disabled:opacity-45"
        >
          <Plus size={15} strokeWidth={2.5} />
          {missing > 0 ? `补 ${missing} 道` : "已够数"}
        </button>
        <button
          onClick={onReplaceMeal}
          disabled={disabled || target === 0}
          className="flex h-10 items-center justify-center gap-1.5 rounded-2xl bg-card text-[12px] font-extrabold text-text2 transition-all hover:text-primary active:scale-95 disabled:opacity-45"
        >
          <Dice5 size={15} strokeWidth={2.5} />
          换一批
        </button>
        <button
          onClick={onOpenPicker}
          disabled={disabled}
          className="flex h-10 items-center justify-center gap-1.5 rounded-2xl bg-card text-[12px] font-extrabold text-text2 transition-all hover:text-mint active:scale-95 disabled:opacity-45"
        >
          <Search size={15} strokeWidth={2.5} />
          手动选
        </button>
      </div>
    </section>
  )
}

function DishPickerModal({
  meal,
  selectedMealById,
  onAdd,
  onRemove,
  onClose,
}: {
  meal: MealType
  selectedMealById: Map<number, MealType>
  onAdd: (dish: Dish) => void
  onRemove: (meal: MealType, dishId: number) => void
  onClose: () => void
}) {
  const [search, setSearch] = useState("")
  const [category, setCategory] = useState("全部")
  const meta = mealMeta[meal]
  const { data, isLoading } = useQuery({
    queryKey: ["dishes", "tomorrow-picker", meal],
    queryFn: () => dishesApi.list({ enabled: "true", meal_type: meal, pageSize: "100", sort: "sort_order", order: "asc" }),
  })
  const dishes = data?.items || []
  const candidates = useMemo(() => dishes.filter((d) => matchesMeal(d, meal)), [dishes, meal])

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
        onClick={(e) => e.stopPropagation()}
        className="relative flex max-h-[86vh] w-full max-w-[640px] flex-col rounded-t-[28px] bg-card shadow-[0_-8px_30px_rgba(0,0,0,.16)] animate-fadeUp"
      >
        <div className="shrink-0 px-5 pt-3">
          <div className="mx-auto mb-4 h-1 w-11 rounded-full bg-border2" />
          <div className="mb-4 flex items-center justify-between gap-3">
            <div className="min-w-0">
              <div className="truncate text-lg font-extrabold">挑选{meta.shortLabel}</div>
              <div className="mt-0.5 text-[12px] font-medium text-text3">{filtered.length} 道可选</div>
            </div>
            <button
              onClick={onClose}
              className="flex h-9 w-9 shrink-0 items-center justify-center rounded-full bg-bg text-text3 transition-all hover:text-primary active:scale-95"
              aria-label="关闭"
            >
              <X size={18} strokeWidth={2.35} />
            </button>
          </div>

          <label className="relative mb-3 block">
            <Search className="absolute left-3.5 top-1/2 -translate-y-1/2 text-text3" size={17} strokeWidth={2.35} />
            <input
              value={search}
              onChange={(e) => setSearch(e.target.value)}
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
              {Array.from({ length: 6 }).map((_, i) => (
                <div key={i} className="h-16 rounded-2xl skeleton" />
              ))}
            </div>
          ) : filtered.length === 0 ? (
            <div className="py-14 text-center">
              <div className="mx-auto mb-3 flex h-12 w-12 items-center justify-center rounded-2xl bg-bg text-text3">
                <Search size={23} strokeWidth={2.35} />
              </div>
              <div className="text-sm font-bold text-text2">没有找到合适的菜</div>
              <div className="mt-1 text-[12px] text-text3">换个关键词或分类试试</div>
            </div>
          ) : (
            <div className="divide-y divide-border/70">
              {filtered.map((dish) => {
                const selectedMeal = selectedMealById.get(dish.id)
                const selectedHere = selectedMeal === meal
                const selectedOther = selectedMeal && selectedMeal !== meal
                return (
                  <div key={dish.id} className="flex items-center gap-3 py-3">
                    <div className="h-14 w-14 shrink-0 overflow-hidden rounded-2xl bg-gradient-to-br from-primary-light to-pink-light">
                      <DishImage dish={dish} className="h-full w-full" emojiSize="text-[27px]" />
                    </div>
                    <div className="min-w-0 flex-1">
                      <div className="truncate text-sm font-extrabold">{dish.name}</div>
                      <div className="mt-1 flex min-w-0 items-center gap-1.5 text-[11px] font-medium text-text3">
                        {dish.category && <span className="max-w-[84px] truncate">{dish.category}</span>}
                        {dish.category && <span className="h-[3px] w-[3px] rounded-full bg-text4" />}
                        <span>{cookTimeText(dish)}</span>
                        <span className="h-[3px] w-[3px] rounded-full bg-text4" />
                        <span>{diffLabel(dish.difficulty)}</span>
                      </div>
                    </div>
                    {selectedOther ? (
                      <button
                        disabled
                        className="h-9 shrink-0 rounded-full bg-bg px-3 text-[11px] font-extrabold text-text3"
                      >
                        已在{mealMeta[selectedMeal].shortLabel}
                      </button>
                    ) : selectedHere ? (
                      <button
                        onClick={() => onRemove(meal, dish.id)}
                        className="flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-full bg-red-light px-3 text-[12px] font-extrabold text-red transition-all active:scale-95"
                      >
                        <X size={13} strokeWidth={2.5} />
                        移除
                      </button>
                    ) : (
                      <button
                        onClick={() => onAdd(dish)}
                        className={`flex h-9 shrink-0 items-center justify-center gap-1.5 rounded-full px-3 text-[12px] font-extrabold text-white transition-all active:scale-95 ${meta.accent}`}
                      >
                        <Plus size={13} strokeWidth={2.7} />
                        加入
                      </button>
                    )}
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
        <div className="h-[194px] rounded-[24px] skeleton" />
        <div className="h-[180px] rounded-[24px] skeleton" />
        <div className="h-[180px] rounded-[24px] skeleton" />
      </div>
    </div>
  )
}

interface SavePlanPayload {
  oldRecords: MealRecord[]
  records: Partial<MealRecord>[]
  wasSaved: boolean
}

export default function Tomorrow() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const confettiRef = useRef<HTMLDivElement>(null)
  const { str: tomorrowStr, label: tomorrowLabel } = getTomorrow()

  const [profile, setProfile] = useState<PlanProfile>("balanced")
  const [targets, setTargets] = useState<Record<MealType, number>>(fallbackTargets)
  const [lunch, setLunch] = useState<Dish[]>([])
  const [dinner, setDinner] = useState<Dish[]>([])
  const [originalRecords, setOriginalRecords] = useState<MealRecord[]>([])
  const [initializedFor, setInitializedFor] = useState<string | null>(null)
  const [pickerMeal, setPickerMeal] = useState<MealType | null>(null)
  const [picking, setPicking] = useState(false)
  const initStartedForRef = useRef<string | null>(null)

  const { data: tomorrowRecords, isLoading: recordsLoading } = useQuery({
    queryKey: ["records", "tomorrow", tomorrowStr],
    queryFn: () => recordsApi.list({ date_from: tomorrowStr, date_to: tomorrowStr, pageSize: "100" }),
  })
  const { data: settings, isLoading: settingsLoading } = useQuery({
    queryKey: ["settings"],
    queryFn: () => settingsApi.get(),
  })

  const defaultTargets = useMemo<Record<MealType, number>>(() => ({
    lunch: targetFromSetting(settings?.lunch_dishes_per_day, fallbackTargets.lunch),
    dinner: targetFromSetting(settings?.dinner_dishes_per_day, fallbackTargets.dinner),
  }), [settings])

  useEffect(() => {
    if (recordsLoading || settingsLoading || initializedFor === tomorrowStr) return
    if (initStartedForRef.current === tomorrowStr) return
    initStartedForRef.current = tomorrowStr

    const records = tomorrowRecords?.items || []
    setOriginalRecords(records)

    if (records.length > 0) {
      const nextLunch = mealRecords(records, "lunch").map(recordToDish)
      const nextDinner = mealRecords(records, "dinner").map(recordToDish)
      setLunch(nextLunch)
      setDinner(nextDinner)
      setTargets({
        lunch: Math.min(maxPerMeal, Math.max(nextLunch.length, defaultTargets.lunch)),
        dinner: Math.min(maxPerMeal, Math.max(nextDinner.length, defaultTargets.dinner)),
      })
      setInitializedFor(tomorrowStr)
      return
    }

    setTargets(defaultTargets)
    void generateAll(profile, defaultTargets).finally(() => setInitializedFor(tomorrowStr))
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [defaultTargets, initializedFor, recordsLoading, settingsLoading, tomorrowRecords, tomorrowStr])

  const selectedMealById = useMemo(() => {
    const map = new Map<number, MealType>()
    lunch.forEach((dish) => map.set(dish.id, "lunch"))
    dinner.forEach((dish) => map.set(dish.id, "dinner"))
    return map
  }, [lunch, dinner])

  const recordsToSave = useMemo<Partial<MealRecord>[]>(() => {
    const records: Partial<MealRecord>[] = []
    lunch.forEach((dish) => records.push({ dish_id: dish.id, dish_name: dish.name, meal_type: "lunch", meal_date: tomorrowStr }))
    dinner.forEach((dish) => records.push({ dish_id: dish.id, dish_name: dish.name, meal_type: "dinner", meal_date: tomorrowStr }))
    return records
  }, [dinner, lunch, tomorrowStr])

  const hasSavedPlan = originalRecords.length > 0
  const totalCount = lunch.length + dinner.length
  const currentKey = keyFromDishes(lunch, dinner)
  const originalKey = keyFromRecords(originalRecords)
  const hasChanges = initializedFor === tomorrowStr && currentKey !== originalKey
  const loading = recordsLoading || settingsLoading || initializedFor !== tomorrowStr
  const disabled = loading || picking

  const savePlanMut = useMutation({
    mutationFn: async ({ oldRecords, records }: SavePlanPayload) => {
      await Promise.all(
        oldRecords.map(async (record) => {
          try {
            await recordsApi.delete(record.id)
          } catch {
            // Stale local records should not block replacing tomorrow's plan.
          }
        }),
      )
      if (records.length === 0) return { created: [] as MealRecord[], skipped: 0, total: 0 }
      return recordsApi.batchCreate(records)
    },
    onSuccess: (res, variables) => {
      setOriginalRecords(res.created)
      qc.invalidateQueries({ queryKey: ["records"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      qc.refetchQueries({ queryKey: ["shopping-list"] })
      launchConfetti(confettiRef.current)
      toast.success(variables.wasSaved ? "明天菜单已更新" : "明天菜单已保存")
    },
    onError: () => toast.error("保存失败，请重试"),
  })

  function getMeal(meal: MealType) {
    return meal === "lunch" ? lunch : dinner
  }

  function updateMeal(meal: MealType, updater: (prev: Dish[]) => Dish[]) {
    if (meal === "lunch") setLunch(updater)
    else setDinner(updater)
  }

  function updateTarget(meal: MealType, value: number) {
    const next = Math.max(0, Math.min(maxPerMeal, value))
    setTargets((prev) => ({ ...prev, [meal]: next }))
    updateMeal(meal, (prev) => (prev.length > next ? prev.slice(0, next) : prev))
  }

  async function requestTomorrowDishes(
    meal: MealType,
    count: number,
    nextProfile = profile,
    excludeIds: number[] = [],
  ) {
    if (count <= 0) return []
    const res = await pickApi.tomorrow({
      meal_type: meal,
      profile: nextProfile,
      count,
      exclude_ids: excludeIds,
    })
    return res.dishes || []
  }

  async function fillMeal(meal: MealType) {
    const current = getMeal(meal)
    const needed = Math.max(0, targets[meal] - current.length)
    if (needed === 0) return
    const otherMeal = meal === "lunch" ? dinner : lunch
    const excludeIds = [
      ...otherMeal.map((dish) => dish.id),
      ...current.map((dish) => dish.id),
    ]
    setPicking(true)
    try {
      const picks = await requestTomorrowDishes(meal, needed, profile, excludeIds)
      if (picks.length === 0) {
        toast.error("没有更多合适的菜了")
        return
      }
      updateMeal(meal, (prev) => uniqueById([...prev, ...picks]).slice(0, maxPerMeal))
    } catch {
      toast.error("没有更多合适的菜了")
    } finally {
      setPicking(false)
    }
  }

  async function replaceMeal(meal: MealType) {
    const count = Math.max(1, targets[meal] || getMeal(meal).length || 1)
    const otherMeal = meal === "lunch" ? dinner : lunch
    setPicking(true)
    try {
      const picks = await requestTomorrowDishes(meal, count, profile, otherMeal.map((dish) => dish.id))
      if (picks.length === 0) {
        toast.error("暂无可推荐的菜品")
        return
      }
      updateMeal(meal, () => picks)
    } catch {
      toast.error("暂无可推荐的菜品")
    } finally {
      setPicking(false)
    }
  }

  async function swapDish(meal: MealType, dishId: number) {
    const current = getMeal(meal)
    const otherMeal = meal === "lunch" ? dinner : lunch
    const keepIds = current.filter((dish) => dish.id !== dishId).map((dish) => dish.id)
    const excludeIds = [...keepIds, ...otherMeal.map((dish) => dish.id), dishId]
    setPicking(true)
    try {
      const [pick] = await requestTomorrowDishes(meal, 1, profile, excludeIds)
      if (!pick) {
        toast.error("没有可替换的菜了")
        return
      }
      updateMeal(meal, (prev) => prev.map((dish) => (dish.id === dishId ? pick : dish)))
    } catch {
      toast.error("没有可替换的菜了")
    } finally {
      setPicking(false)
    }
  }

  function addManual(meal: MealType, dish: Dish) {
    const selectedMeal = selectedMealById.get(dish.id)
    if (selectedMeal) {
      toast.error(`这道菜已经在${mealMeta[selectedMeal].shortLabel}里了`)
      return
    }
    if (getMeal(meal).length >= maxPerMeal) {
      toast.error(`每餐最多 ${maxPerMeal} 道菜`)
      return
    }
    updateMeal(meal, (prev) => [...prev, dish])
    setTargets((prev) => ({ ...prev, [meal]: Math.max(prev[meal], getMeal(meal).length + 1) }))
  }

  function removeDish(meal: MealType, dishId: number) {
    updateMeal(meal, (prev) => prev.filter((dish) => dish.id !== dishId))
  }

  async function generateAll(nextProfile = profile, nextTargets = targets) {
    setPicking(true)
    try {
      const nextLunch = nextTargets.lunch > 0
        ? await requestTomorrowDishes("lunch", nextTargets.lunch, nextProfile)
        : []
      const nextDinner = nextTargets.dinner > 0
        ? await requestTomorrowDishes("dinner", nextTargets.dinner, nextProfile, nextLunch.map((dish) => dish.id))
        : []
      if (nextLunch.length === 0 && nextDinner.length === 0) {
        toast.error("暂无可推荐的菜品")
        return
      }
      setLunch(nextLunch)
      setDinner(nextDinner)
    } catch {
      toast.error("暂无可推荐的菜品")
    } finally {
      setPicking(false)
    }
  }

  function applyProfile(nextProfile: PlanProfile) {
    setProfile(nextProfile)
    void generateAll(nextProfile)
  }

  function savePlan() {
    if (recordsToSave.length === 0) {
      toast.error("至少选一道菜")
      return
    }
    if (hasSavedPlan && !hasChanges) {
      toast("菜单没有变化")
      return
    }
    savePlanMut.mutate({ oldRecords: originalRecords, records: recordsToSave, wasSaved: hasSavedPlan })
  }

  return (
    <div className="animate-fadeUp pb-[116px]">
      {createPortal(<div ref={confettiRef} className="fixed inset-0 z-50 pointer-events-none overflow-hidden" />, document.body)}

      <PageHeader
        title="明天吃什么"
        subtitle={tomorrowLabel}
        icon={CalendarClock}
        onBack={() => navigate(-1)}
      />

      {loading ? (
        <LoadingState />
      ) : (
        <main className="px-5 py-4">
          <div className="mx-auto max-w-[640px] space-y-4">
            <PlanSummary
              dateLabel={tomorrowLabel}
              totalCount={totalCount}
              hasSavedPlan={hasSavedPlan}
              hasChanges={hasChanges}
              lunch={lunch}
              dinner={dinner}
              onGenerateAll={() => generateAll()}
              onOpenShopping={() => navigate("/more")}
              disabled={disabled}
            />

            <section>
              <div className="mb-3 flex items-center justify-between gap-3">
                <div>
                  <div className="text-[15px] font-extrabold">偏好</div>
                  <div className="text-[11px] font-medium text-text3">生成和替换都会按这里来</div>
                </div>
                <button
                  onClick={() => generateAll()}
                  disabled={disabled}
                  className="flex h-9 shrink-0 items-center gap-1.5 rounded-full bg-bg px-3 text-[12px] font-extrabold text-text2 transition-all hover:text-primary active:scale-95 disabled:opacity-45"
                >
                  <Dice5 size={14} strokeWidth={2.5} />
                  试一版
                </button>
              </div>
              <div className="flex gap-2 overflow-x-auto pb-1 scrollbar-none">
                {profileOptions.map((option) => (
                  <ProfileButton
                    key={option.key}
                    option={option}
                    active={profile === option.key}
                    onClick={() => applyProfile(option.key)}
                  />
                ))}
              </div>
            </section>

            <MealPlanner
              meal="lunch"
              dishes={lunch}
              target={targets.lunch}
              disabled={disabled}
              onTargetChange={(value) => updateTarget("lunch", value)}
              onOpenDish={(dish) => navigate(`/dishes/${dish.id}`)}
              onFill={() => fillMeal("lunch")}
              onReplaceMeal={() => replaceMeal("lunch")}
              onSwapDish={(dishId) => swapDish("lunch", dishId)}
              onRemoveDish={(dishId) => removeDish("lunch", dishId)}
              onOpenPicker={() => setPickerMeal("lunch")}
            />

            <MealPlanner
              meal="dinner"
              dishes={dinner}
              target={targets.dinner}
              disabled={disabled}
              onTargetChange={(value) => updateTarget("dinner", value)}
              onOpenDish={(dish) => navigate(`/dishes/${dish.id}`)}
              onFill={() => fillMeal("dinner")}
              onReplaceMeal={() => replaceMeal("dinner")}
              onSwapDish={(dishId) => swapDish("dinner", dishId)}
              onRemoveDish={(dishId) => removeDish("dinner", dishId)}
              onOpenPicker={() => setPickerMeal("dinner")}
            />
          </div>
        </main>
      )}

      {!loading && (
        <div
          className="fixed bottom-0 left-0 right-0 z-[110] border-t border-white/70 bg-bg/92 px-5 pt-3 backdrop-blur-2xl shadow-[0_-10px_28px_rgba(26,26,46,.07)]"
          style={{ paddingBottom: "calc(12px + env(safe-area-inset-bottom))" }}
        >
          <div className="mx-auto flex max-w-[640px] items-center gap-3">
            <div className="min-w-0 flex-1">
              <div className="truncate text-[12px] font-bold text-text2">
                {totalCount > 0 ? `${tomorrowLabel} · ${totalCount} 道菜` : "还没有选择菜品"}
              </div>
              <div className="truncate text-[11px] font-medium text-text3">
                {hasSavedPlan ? (hasChanges ? "保存后会覆盖原明日菜单" : "当前菜单已同步到买菜清单") : "保存后自动生成买菜清单"}
              </div>
            </div>
            <button
              onClick={savePlan}
              disabled={savePlanMut.isPending || totalCount === 0 || (hasSavedPlan && !hasChanges)}
              className="flex h-12 min-w-[148px] items-center justify-center gap-2 rounded-full bg-primary px-5 text-sm font-extrabold text-white transition-all hover:bg-primary-dark active:scale-95 disabled:opacity-50"
            >
              <Check size={17} strokeWidth={2.6} />
              {savePlanMut.isPending ? "保存中" : hasSavedPlan ? (hasChanges ? "更新菜单" : "已保存") : "保存菜单"}
            </button>
          </div>
        </div>
      )}

      {pickerMeal && (
        <DishPickerModal
          meal={pickerMeal}
          selectedMealById={selectedMealById}
          onAdd={(dish) => addManual(pickerMeal, dish)}
          onRemove={(meal, dishId) => removeDish(meal, dishId)}
          onClose={() => setPickerMeal(null)}
        />
      )}
    </div>
  )
}
