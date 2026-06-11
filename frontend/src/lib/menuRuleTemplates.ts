import type { MenuRule } from "@/types"
import { PROTEIN_OPTIONS } from "@/lib/weekPlanCombos"

// Mirrors the backend MenuRuleTemplate JSON schema; the backend re-renders
// the executable expression from this on save/validate.
export interface MenuRuleTemplate {
  type: "limit" | "no_repeat" | "prefer" | "avoid"
  scope?: "meal" | "day" | "week"
  category?: string
  n?: number
  points?: number
  strength?: "must" | "prefer"
}

export const TYPE_OPTIONS: Array<{ key: MenuRuleTemplate["type"]; label: string }> = [
  { key: "limit", label: "最多出现" },
  { key: "no_repeat", label: "主蛋白不重样" },
  { key: "prefer", label: "优先安排" },
  { key: "avoid", label: "尽量避开" },
]

export const SCOPE_OPTIONS: Array<{ key: "meal" | "day" | "week"; label: string }> = [
  { key: "meal", label: "同一餐" },
  { key: "day", label: "同一天" },
  { key: "week", label: "一周" },
]

export const STRENGTH_OPTIONS: Array<{ key: "must" | "prefer"; label: string }> = [
  { key: "prefer", label: "尽量做到" },
  { key: "must", label: "必须遵守" },
]

const FIXED_CATEGORY_LABELS: Record<string, string> = {
  egg: "蛋类菜",
  cold: "凉菜",
  staple: "主食",
  soup: "汤",
  spicy: "辣菜",
  heavy_spicy: "重油重辣的菜",
  deep_fry: "油炸菜",
  slow: "费时菜（超过45分钟）",
}

// Display-only categories used by the v3/v4 preset rules. Not offered in the
// sentence builder — they exist for rendering the factory rules' sentences.
const DISPLAY_ONLY_CATEGORY_LABELS: Record<string, string> = {
  non_favorite: "没收藏的菜",
  unfamiliar_category: "一道收藏都没有的菜系",
  weekday_slow_soup: "工作日的费时汤",
  slow_soup: "费时汤（炖煮超45分钟）",
  soup_ingredient_repeat: "和昨天的汤撞主料的汤",
}

export const CATEGORY_OPTIONS: Array<{ key: string; label: string }> = [
  ...Object.entries(FIXED_CATEGORY_LABELS).map(([key, label]) => ({ key, label })),
  ...PROTEIN_OPTIONS.map((option) => ({ key: `protein:${option.key}`, label: `${option.label}菜` })),
]

export function categoryLabel(category?: string): string {
  if (!category) return "某类菜"
  if (category.startsWith("protein:")) {
    const key = category.slice("protein:".length)
    const label = PROTEIN_OPTIONS.find((option) => option.key === key)?.label || key
    return `${label}菜`
  }
  if (category.startsWith("ingredient:")) {
    return `带「${category.slice("ingredient:".length)}」的菜`
  }
  return FIXED_CATEGORY_LABELS[category] || DISPLAY_ONLY_CATEGORY_LABELS[category] || category
}

const SCOPE_LABELS: Record<string, string> = { meal: "同一餐", day: "同一天", week: "一周" }

export function describeTemplate(tpl: MenuRuleTemplate): string {
  const scope = SCOPE_LABELS[tpl.scope || "meal"]
  const strength = tpl.strength === "must" ? "必须遵守" : "尽量做到"
  const n = tpl.n && tpl.n > 0 ? tpl.n : 1
  switch (tpl.type) {
    case "limit":
      return `${scope}里，${categoryLabel(tpl.category)}最多 ${n} 道 · ${strength}`
    case "no_repeat":
      if ((tpl.scope || "meal") === "meal" && n <= 1) {
        return `同一餐里，主蛋白尽量不重样（豆制品除外） · ${strength}`
      }
      return `${scope}里，同一种主蛋白最多安排 ${n} 次（豆制品除外） · ${strength}`
    case "prefer":
      return `优先安排${categoryLabel(tpl.category)}`
    case "avoid":
      return `尽量避开${categoryLabel(tpl.category)}`
    default:
      return ""
  }
}

export function parseTemplate(rule: MenuRule): MenuRuleTemplate | null {
  const raw = rule.template?.trim()
  if (!raw) return null
  try {
    const parsed = JSON.parse(raw) as MenuRuleTemplate
    if (!parsed || typeof parsed !== "object") return null
    if (!TYPE_OPTIONS.some((option) => option.key === parsed.type)) return null
    return parsed
  } catch {
    return null
  }
}

// Updates only the template; the backend overwrites expression/kind/severity
// from it on save, so the stale expression here is harmless.
export function withTemplate(rule: MenuRule, tpl: MenuRuleTemplate): MenuRule {
  return { ...rule, template: JSON.stringify(tpl) }
}
