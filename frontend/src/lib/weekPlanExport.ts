import type { WeekPlan, WeekPlanPreferences } from "@/types"

const mealLabels = { lunch: "午餐", dinner: "晚餐" } as const
const periodLabels = { weekday: "工作日", weekend: "周末" } as const
const profileLabels = {
  balanced: "均衡",
  quick: "快手",
  light: "清淡",
  spicy: "想吃辣",
  favorite: "收藏优先",
} as const

const palette = {
  paper: "#FFF7F0",
  card: "#FFFFFF",
  ink: "#202036",
  muted: "#7E8194",
  faint: "#A7A8B7",
  line: "#F0DCD1",
  coral: "#E8734A",
  coralSoft: "#FFE3D6",
  green: "#2FAF83",
  greenSoft: "#DDF6EA",
  blue: "#3D6C8F",
  blueSoft: "#EAF3F8",
  gold: "#B8792F",
  goldSoft: "#FFF0D4",
}

type MealKey = keyof typeof mealLabels
type PeriodKey = keyof typeof periodLabels

type MealBlock = {
  meal: MealKey
  dishes: string[]
  rows: ChipRow[]
  height: number
}

type DayCard = {
  day: WeekPlan["days"][number]
  meals: MealBlock[]
  height: number
}

type ChipRow = Array<{ text: string; width: number }>

type Layout = {
  cards: DayCard[]
  height: number
}

export function exportWeekPlanAsPng(plan: WeekPlan, prefs: WeekPlanPreferences) {
  const days = plan.days || []
  const fileName = `ninimenu-week-plan-${days[0]?.date || todayString()}-${timeString()}.png`
  const width = 1240
  const margin = 56
  const columnGap = 24
  const columnWidth = (width - margin * 2 - columnGap) / 2
  const headerHeight = 348
  const footerHeight = 64

  const canvas = document.createElement("canvas")
  const scale = Math.max(1, Math.min(2, window.devicePixelRatio || 1))
  const ctx = canvas.getContext("2d")
  if (!ctx) return

  ctx.font = font(24, 700)
  const layout = buildLayout(ctx, plan, columnWidth, headerHeight, footerHeight, margin)

  canvas.width = width * scale
  canvas.height = layout.height * scale
  canvas.style.width = `${width}px`
  canvas.style.height = `${layout.height}px`
  ctx.scale(scale, scale)

  drawBackground(ctx, width, layout.height)
  drawHeader(ctx, plan, prefs, width, margin)
  drawCards(ctx, layout.cards, margin, headerHeight, columnWidth, columnGap)
  drawFooter(ctx, width, layout.height, margin)

  const link = document.createElement("a")
  link.download = fileName
  link.href = canvas.toDataURL("image/png")
  link.click()

  return fileName
}

function buildLayout(
  ctx: CanvasRenderingContext2D,
  plan: WeekPlan,
  columnWidth: number,
  headerHeight: number,
  footerHeight: number,
  margin: number,
): Layout {
  const cards = (plan.days || []).map((day) => buildDayCard(ctx, day, columnWidth))
  const columnHeights = [0, 0]
  cards.forEach((card, index) => {
    const column = index % 2
    columnHeights[column] += card.height + 24
  })
  const gridHeight = Math.max(columnHeights[0], columnHeights[1]) - (cards.length > 0 ? 24 : 0)
  return {
    cards,
    height: Math.max(900, headerHeight + gridHeight + footerHeight + margin),
  }
}

function buildDayCard(ctx: CanvasRenderingContext2D, day: WeekPlan["days"][number], width: number): DayCard {
  const contentWidth = width - 48
  const meals: MealBlock[] = [
    ...(day.lunch.length > 0 ? [buildMealBlock(ctx, "lunch", day.lunch.map((dish) => dish.name), contentWidth)] : []),
    ...(day.dinner.length > 0 ? [buildMealBlock(ctx, "dinner", day.dinner.map((dish) => dish.name), contentWidth)] : []),
  ]
  const mealsHeight = meals.reduce((sum, meal) => sum + meal.height, 0) + Math.max(0, meals.length - 1) * 14
  return {
    day,
    meals,
    height: Math.max(176, 86 + (meals.length === 0 ? 54 : mealsHeight) + 28),
  }
}

function buildMealBlock(ctx: CanvasRenderingContext2D, meal: MealKey, dishes: string[], width: number): MealBlock {
  ctx.font = font(21, 700)
  const rows = wrapChips(ctx, dishes, width)
  return {
    meal,
    dishes,
    rows,
    height: 36 + rows.length * 40,
  }
}

function wrapChips(ctx: CanvasRenderingContext2D, names: string[], width: number) {
  const rows: ChipRow[] = []
  let row: ChipRow = []
  let rowWidth = 0
  names.forEach((name) => {
    const chipWidth = Math.min(width, Math.ceil(ctx.measureText(name).width) + 30)
    const gap = row.length > 0 ? 10 : 0
    if (row.length > 0 && rowWidth + gap + chipWidth > width) {
      rows.push(row)
      row = []
      rowWidth = 0
    }
    row.push({ text: name, width: chipWidth })
    rowWidth += (row.length > 1 ? 10 : 0) + chipWidth
  })
  if (row.length > 0) rows.push(row)
  return rows
}

function drawBackground(ctx: CanvasRenderingContext2D, width: number, height: number) {
  ctx.fillStyle = palette.paper
  ctx.fillRect(0, 0, width, height)

  ctx.fillStyle = "rgba(232, 115, 74, 0.08)"
  for (let x = 34; x < width; x += 46) {
    for (let y = 36; y < height; y += 46) {
      ctx.beginPath()
      ctx.arc(x, y, 1.4, 0, Math.PI * 2)
      ctx.fill()
    }
  }
}

function drawHeader(ctx: CanvasRenderingContext2D, plan: WeekPlan, prefs: WeekPlanPreferences, width: number, margin: number) {
  const top = 42
  const headerWidth = width - margin * 2
  const headerInnerX = margin + 30
  const preferenceGap = 20
  const preferenceWidth = (headerWidth - 60 - preferenceGap) / 2
  drawRoundRect(ctx, margin, top, headerWidth, 262, 34, palette.card)
  strokeRoundRect(ctx, margin, top, headerWidth, 262, 34, "rgba(240, 220, 209, 0.95)", 2)

  drawPill(ctx, headerInnerX, top + 28, "NiniMenu", palette.ink, "#FFFFFF", 154, 42, 20, 800)
  ctx.fillStyle = palette.ink
  ctx.font = font(52, 850)
  ctx.fillText("一周菜单", headerInnerX, top + 106)
  ctx.fillStyle = palette.muted
  ctx.font = font(24, 650)
  ctx.fillText(dateRangeText(plan), headerInnerX + 4, top + 150)

  const total = dishCount(plan)
  const skipped = skippedMealCount(plan)
  drawMetric(ctx, width - margin - 286, top + 32, "总菜数", `${total}`, palette.coral, palette.coralSoft)
  drawMetric(ctx, width - margin - 144, top + 32, "已跳过", `${skipped}`, palette.green, palette.greenSoft)

  drawPreferenceCard(ctx, headerInnerX, top + 176, "weekday", prefs.weekday, preferenceWidth, palette.coral, palette.coralSoft)
  drawPreferenceCard(ctx, headerInnerX + preferenceWidth + preferenceGap, top + 176, "weekend", prefs.weekend, preferenceWidth, palette.green, palette.greenSoft)
}

function drawMetric(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  label: string,
  value: string,
  color: string,
  bg: string,
) {
  drawRoundRect(ctx, x, y, 112, 78, 24, bg)
  ctx.fillStyle = color
  ctx.font = font(19, 800)
  ctx.fillText(label, x + 18, y + 28)
  ctx.fillStyle = palette.ink
  ctx.font = font(32, 850)
  ctx.fillText(value, x + 18, y + 62)
}

function drawPreferenceCard(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  period: PeriodKey,
  pref: WeekPlanPreferences[PeriodKey],
  width: number,
  color: string,
  bg: string,
) {
  drawRoundRect(ctx, x, y, width, 64, 22, bg)
  ctx.fillStyle = color
  ctx.font = font(20, 850)
  ctx.fillText(periodLabels[period], x + 18, y + 26)
  ctx.fillStyle = palette.ink
  ctx.font = font(20, 800)
  ctx.fillText(profileLabels[pref.profile], x + 92, y + 26)
  ctx.fillStyle = color
  ctx.font = font(18, 750)
  ctx.fillText(`午 ${quotaText(pref.lunch)}`, x + 18, y + 52)
  ctx.fillText(`晚 ${quotaText(pref.dinner)}`, x + 238, y + 52)
}

function drawCards(
  ctx: CanvasRenderingContext2D,
  cards: DayCard[],
  margin: number,
  startY: number,
  columnWidth: number,
  columnGap: number,
) {
  const columnY = [startY, startY]
  cards.forEach((card, index) => {
    const column = index % 2
    const x = margin + column * (columnWidth + columnGap)
    drawDayCard(ctx, card, x, columnY[column], columnWidth, index)
    columnY[column] += card.height + 24
  })
}

function drawDayCard(ctx: CanvasRenderingContext2D, card: DayCard, x: number, y: number, width: number, index: number) {
  drawRoundRect(ctx, x, y, width, card.height, 28, palette.card)
  strokeRoundRect(ctx, x, y, width, card.height, 28, "rgba(240, 220, 209, 0.92)", 2)

  const accent = index >= 5 ? palette.green : palette.coral
  drawRoundRect(ctx, x, y, 8, card.height, 4, accent)

  ctx.fillStyle = palette.ink
  ctx.font = font(35, 850)
  ctx.fillText(card.day.day_name, x + 24, y + 48)
  ctx.fillStyle = palette.faint
  ctx.font = font(22, 750)
  const date = card.day.date.slice(5)
  ctx.fillText(date, x + width - 24 - ctx.measureText(date).width, y + 44)

  const total = card.day.lunch.length + card.day.dinner.length
  drawSmallTag(ctx, x + 24, y + 64, `${total} 道`, palette.blueSoft, palette.blue)

  if (card.meals.length === 0) {
    ctx.fillStyle = palette.muted
    ctx.font = font(23, 700)
    ctx.fillText("当天不推荐", x + 24, y + 126)
    return
  }

  let mealY = y + 98
  card.meals.forEach((meal, mealIndex) => {
    if (mealIndex > 0) {
      ctx.strokeStyle = "rgba(240, 220, 209, 0.75)"
      ctx.lineWidth = 1.5
      ctx.beginPath()
      ctx.moveTo(x + 24, mealY - 12)
      ctx.lineTo(x + width - 24, mealY - 12)
      ctx.stroke()
    }
    drawMealBlock(ctx, meal, x + 24, mealY, width - 48)
    mealY += meal.height + 14
  })
}

function drawMealBlock(ctx: CanvasRenderingContext2D, block: MealBlock, x: number, y: number, width: number) {
  const color = block.meal === "lunch" ? palette.coral : palette.green
  const soft = block.meal === "lunch" ? palette.coralSoft : palette.greenSoft
  drawSmallTag(ctx, x, y, mealLabels[block.meal], soft, color)

  ctx.fillStyle = palette.muted
  ctx.font = font(18, 700)
  const countText = `${block.dishes.length} 道`
  ctx.fillText(countText, x + width - ctx.measureText(countText).width, y + 24)

  let rowY = y + 42
  block.rows.forEach((row) => {
    let chipX = x
    row.forEach((chip) => {
      drawRoundRect(ctx, chipX, rowY, chip.width, 30, 15, "#FFFDFC")
      strokeRoundRect(ctx, chipX, rowY, chip.width, 30, 15, "rgba(229, 214, 205, 0.85)", 1)
      ctx.fillStyle = palette.ink
      ctx.font = font(20, 750)
      ctx.fillText(chip.text, chipX + 15, rowY + 22)
      chipX += chip.width + 10
    })
    rowY += 40
  })
}

function drawFooter(ctx: CanvasRenderingContext2D, width: number, height: number, margin: number) {
  ctx.fillStyle = palette.faint
  ctx.font = font(18, 650)
  ctx.fillText("Generated by NiniMenu", margin, height - 32)
  const text = todayString()
  ctx.fillText(text, width - margin - ctx.measureText(text).width, height - 32)
}

function drawPill(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  text: string,
  bg: string,
  color: string,
  minWidth: number,
  height: number,
  size: number,
  weight: number,
) {
  ctx.font = font(size, weight)
  const pillWidth = Math.max(minWidth, ctx.measureText(text).width + 34)
  drawRoundRect(ctx, x, y, pillWidth, height, height / 2, bg)
  ctx.fillStyle = color
  ctx.fillText(text, x + 17, y + Math.round(height * 0.68))
}

function drawSmallTag(ctx: CanvasRenderingContext2D, x: number, y: number, text: string, bg: string, color: string) {
  ctx.font = font(20, 850)
  const width = Math.ceil(ctx.measureText(text).width) + 30
  drawRoundRect(ctx, x, y, width, 32, 16, bg)
  ctx.fillStyle = color
  ctx.fillText(text, x + 15, y + 23)
}

function quotaText(quota: { meat_count: number; veg_count: number; soup_count: number }) {
  const total = quota.meat_count + quota.veg_count + quota.soup_count
  if (total === 0) return "跳过"
  return `${quota.meat_count}荤 ${quota.veg_count}素 ${quota.soup_count}汤`
}

function dishCount(plan: WeekPlan) {
  return (plan.days || []).reduce((sum, day) => sum + day.lunch.length + day.dinner.length, 0)
}

function skippedMealCount(plan: WeekPlan) {
  return (plan.days || []).reduce((sum, day) => sum + (day.lunch.length === 0 ? 1 : 0) + (day.dinner.length === 0 ? 1 : 0), 0)
}

function dateRangeText(plan: WeekPlan) {
  const days = plan.days || []
  if (days.length === 0) return "暂无日期"
  const first = days[0]?.date
  const last = days[days.length - 1]?.date
  return first && last ? `${first} - ${last}` : first || "暂无日期"
}

function todayString() {
  const d = new Date()
  return `${d.getFullYear()}-${String(d.getMonth() + 1).padStart(2, "0")}-${String(d.getDate()).padStart(2, "0")}`
}

function timeString() {
  const d = new Date()
  return `${String(d.getHours()).padStart(2, "0")}${String(d.getMinutes()).padStart(2, "0")}`
}

function font(size: number, weight: number) {
  return `${weight} ${size}px PingFang SC, Hiragino Sans GB, Microsoft YaHei, sans-serif`
}

function drawRoundRect(ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number, fill: string) {
  ctx.beginPath()
  ctx.roundRect(x, y, width, height, radius)
  ctx.fillStyle = fill
  ctx.fill()
}

function strokeRoundRect(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  width: number,
  height: number,
  radius: number,
  stroke: string,
  lineWidth: number,
) {
  ctx.beginPath()
  ctx.roundRect(x, y, width, height, radius)
  ctx.strokeStyle = stroke
  ctx.lineWidth = lineWidth
  ctx.stroke()
}
