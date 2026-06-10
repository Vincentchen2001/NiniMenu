import type { DayOverride, MealQuota, PlanProfile, WeekPlanPreferences } from "@/types"

export const PROTEIN_OPTIONS: Array<{ key: string; label: string }> = [
  { key: "pork", label: "猪肉" },
  { key: "beef", label: "牛肉" },
  { key: "lamb", label: "羊肉" },
  { key: "poultry", label: "鸡鸭" },
  { key: "seafood", label: "鱼虾" },
  { key: "egg", label: "蛋类" },
  { key: "soy", label: "豆制品" },
]

export const PROFILE_EMOJI: Record<PlanProfile, string> = {
  balanced: "✨",
  quick: "⚡",
  light: "🍃",
  spicy: "🌶️",
  favorite: "❤️",
  soup: "🍲",
}

export const PROFILE_LABELS: Record<PlanProfile, string> = {
  balanced: "均衡",
  quick: "快手",
  light: "清淡",
  spicy: "想吃辣",
  favorite: "收藏",
  soup: "靓汤",
}

export const WEEKDAY_KEYS = ["mon", "tue", "wed", "thu", "fri"] as const
export const WEEKEND_KEYS = ["sat", "sun"] as const
export const DAY_KEYS: string[] = [...WEEKDAY_KEYS, ...WEEKEND_KEYS]

export const DAY_LABELS: Record<string, string> = {
  mon: "一", tue: "二", wed: "三", thu: "四", fri: "五", sat: "六", sun: "日",
}

export const DAY_FULL_LABELS: Record<string, string> = {
  mon: "周一", tue: "周二", wed: "周三", thu: "周四", fri: "周五", sat: "周六", sun: "周日",
}

export interface ComboPack {
  key: string
  label: string
  desc: string
  days: Record<string, DayOverride>
}

export const WEEKDAY_PACKS: ComboPack[] = [
  {
    key: "busy_worker",
    label: "上班族快手",
    desc: "周一到周五全走快手菜，下班半小时开饭",
    days: {
      mon: { profile: "quick" },
      tue: { profile: "quick" },
      wed: { profile: "quick" },
      thu: { profile: "quick" },
      fri: { profile: "quick" },
    },
  },
  {
    key: "homely_rhythm",
    label: "家常有节奏",
    desc: "周一清淡开局，周三吃辣提神，周五收藏菜犒劳",
    days: {
      mon: { profile: "light" },
      wed: { profile: "spicy" },
      fri: { profile: "favorite" },
    },
  },
  {
    key: "light_fit",
    label: "清爽减脂",
    desc: "工作日全清淡，周三多吃鱼虾、周五多吃鸡鸭",
    days: {
      mon: { profile: "light" },
      tue: { profile: "light" },
      wed: { profile: "light", want: ["seafood"] },
      thu: { profile: "light" },
      fri: { profile: "light", want: ["poultry"] },
    },
  },
  {
    key: "craving",
    label: "解馋重口",
    desc: "周二、周五安排辣口，过瘾又不连着吃",
    days: {
      tue: { profile: "spicy" },
      fri: { profile: "spicy" },
    },
  },
]

export const WEEKEND_PACKS: ComboPack[] = [
  {
    key: "family_kids",
    label: "亲子时光",
    desc: "周六炖靓汤，周日清淡，适合跟孩子一起吃",
    days: {
      sat: { profile: "soup" },
      sun: { profile: "light" },
    },
  },
  {
    key: "treat_self",
    label: "犒劳自己",
    desc: "周六吃收藏的最爱，周日安排想吃的辣",
    days: {
      sat: { profile: "favorite" },
      sun: { profile: "spicy" },
    },
  },
  {
    key: "soup_week",
    label: "靓汤滋补",
    desc: "周六周日都煲汤，喝个舒坦",
    days: {
      sat: { profile: "soup" },
      sun: { profile: "soup" },
    },
  },
  {
    key: "easy_rest",
    label: "简单休息",
    desc: "周末不折腾，快手菜轻松搞定",
    days: {
      sat: { profile: "quick" },
      sun: { profile: "quick" },
    },
  },
]

function cloneOverride(override: DayOverride): DayOverride {
  return {
    ...(override.profile ? { profile: override.profile } : {}),
    ...(override.want?.length ? { want: [...override.want] } : {}),
  }
}

// applyPacks overwrites the pack's half of the week (weekday pack clears
// mon-fri, weekend pack clears sat-sun) and leaves the other half untouched.
export function applyPacks(
  prefs: WeekPlanPreferences,
  weekdayPack: ComboPack | null,
  weekendPack: ComboPack | null,
): WeekPlanPreferences {
  const days: Record<string, DayOverride> = { ...(prefs.days || {}) }
  if (weekdayPack) {
    WEEKDAY_KEYS.forEach((key) => { delete days[key] })
    Object.entries(weekdayPack.days).forEach(([key, override]) => {
      days[key] = cloneOverride(override)
    })
  }
  if (weekendPack) {
    WEEKEND_KEYS.forEach((key) => { delete days[key] })
    Object.entries(weekendPack.days).forEach(([key, override]) => {
      days[key] = cloneOverride(override)
    })
  }
  return { ...prefs, days }
}

interface PeriodQuotas {
  lunch: MealQuota
  dinner: MealQuota
}

export interface MealCountPreset {
  key: string
  label: string
  desc: string
  weekday: PeriodQuotas
  weekend: PeriodQuotas
}

export const MEAL_COUNT_PRESETS: MealCountPreset[] = [
  {
    key: "couple",
    label: "二人简餐",
    desc: "午 1荤1素 · 晚 1荤1汤",
    weekday: {
      lunch: { meat_count: 1, veg_count: 1, soup_count: 0 },
      dinner: { meat_count: 1, veg_count: 0, soup_count: 1 },
    },
    weekend: {
      lunch: { meat_count: 1, veg_count: 1, soup_count: 0 },
      dinner: { meat_count: 1, veg_count: 1, soup_count: 1 },
    },
  },
  {
    key: "family3",
    label: "三口之家",
    desc: "午 1荤1素 · 晚 2荤1素1汤",
    weekday: {
      lunch: { meat_count: 1, veg_count: 1, soup_count: 0 },
      dinner: { meat_count: 2, veg_count: 1, soup_count: 1 },
    },
    weekend: {
      lunch: { meat_count: 2, veg_count: 1, soup_count: 1 },
      dinner: { meat_count: 2, veg_count: 1, soup_count: 1 },
    },
  },
  {
    key: "gathering",
    label: "人多聚餐",
    desc: "午 2荤1素1汤 · 晚 2荤2素1汤",
    weekday: {
      lunch: { meat_count: 2, veg_count: 1, soup_count: 1 },
      dinner: { meat_count: 2, veg_count: 2, soup_count: 1 },
    },
    weekend: {
      lunch: { meat_count: 2, veg_count: 2, soup_count: 1 },
      dinner: { meat_count: 2, veg_count: 2, soup_count: 1 },
    },
  },
]
