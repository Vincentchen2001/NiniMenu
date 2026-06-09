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

export function exportWeekPlanAsPng(plan: WeekPlan, prefs: WeekPlanPreferences) {
  const width = 1080
  const days = plan.days || []
  const rows = days.map((day) => {
    const meals = [
      ...(day.lunch.length > 0 ? [{ meal: "lunch" as const, lines: splitNames(day.lunch.map((dish) => dish.name)) }] : []),
      ...(day.dinner.length > 0 ? [{ meal: "dinner" as const, lines: splitNames(day.dinner.map((dish) => dish.name)) }] : []),
    ]
    return {
      day,
      meals,
      height: Math.max(118, 84 + meals.reduce((sum, item) => sum + 38 + Math.max(item.lines.length, 1) * 24, 0)),
    }
  })
  const height = 244 + rows.reduce((sum, row) => sum + row.height + 18, 0)
  const canvas = document.createElement("canvas")
  const scale = Math.max(1, Math.min(2, window.devicePixelRatio || 1))
  canvas.width = width * scale
  canvas.height = height * scale
  canvas.style.width = `${width}px`
  canvas.style.height = `${height}px`

  const ctx = canvas.getContext("2d")
  if (!ctx) return
  ctx.scale(scale, scale)
  ctx.fillStyle = "#FFF8F3"
  ctx.fillRect(0, 0, width, height)

  ctx.fillStyle = "#1A1A2E"
  ctx.font = "800 46px PingFang SC, Microsoft YaHei, sans-serif"
  ctx.fillText("NiniMenu 一周菜单", 64, 76)
  ctx.fillStyle = "#7A7A8C"
  ctx.font = "500 24px PingFang SC, Microsoft YaHei, sans-serif"
  ctx.fillText(dateRangeText(plan), 66, 116)

  drawPill(ctx, 64, 148, preferenceText("weekday", prefs.weekday), "#FFE8DC", "#E8734A")
  drawPill(ctx, 420, 148, preferenceText("weekend", prefs.weekend), "#DFF5EC", "#39B980")

  let y = 210
  rows.forEach((row, index) => {
    drawDay(ctx, row, 64, y, width - 128)
    y += row.height + (index === rows.length - 1 ? 0 : 18)
  })

  const link = document.createElement("a")
  link.download = `ninimenu-week-plan-${days[0]?.date || todayString()}.png`
  link.href = canvas.toDataURL("image/png")
  link.click()
}

function drawDay(
  ctx: CanvasRenderingContext2D,
  row: { day: WeekPlan["days"][number]; meals: Array<{ meal: keyof typeof mealLabels; lines: string[] }>; height: number },
  x: number,
  y: number,
  width: number,
) {
  roundRect(ctx, x, y, width, row.height, 26, "#FFFFFF")
  ctx.strokeStyle = "#F1DFD5"
  ctx.lineWidth = 2
  strokeRoundRect(ctx, x, y, width, row.height, 26)

  ctx.fillStyle = "#1A1A2E"
  ctx.font = "800 30px PingFang SC, Microsoft YaHei, sans-serif"
  ctx.fillText(row.day.day_name, x + 28, y + 48)
  ctx.fillStyle = "#A0A0B0"
  ctx.font = "600 21px PingFang SC, Microsoft YaHei, sans-serif"
  ctx.fillText(row.day.date.slice(5), x + width - 100, y + 48)

  if (row.meals.length === 0) {
    ctx.fillStyle = "#A0A0B0"
    ctx.font = "600 22px PingFang SC, Microsoft YaHei, sans-serif"
    ctx.fillText("当天不推荐", x + 28, y + 92)
    return
  }

  let mealY = y + 84
  row.meals.forEach((item) => {
    drawMeal(ctx, item.meal, item.lines, x + 28, mealY, width - 56)
    mealY += 38 + Math.max(item.lines.length, 1) * 24
  })
}

function drawMeal(
  ctx: CanvasRenderingContext2D,
  meal: keyof typeof mealLabels,
  lines: string[],
  x: number,
  y: number,
  width: number,
) {
  const color = meal === "lunch" ? "#E8734A" : "#39B980"
  const soft = meal === "lunch" ? "#FFE8DC" : "#DFF5EC"
  drawPill(ctx, x, y - 23, mealLabels[meal], soft, color, 76)
  ctx.fillStyle = "#3D3D52"
  ctx.font = "600 22px PingFang SC, Microsoft YaHei, sans-serif"
  lines.forEach((line, index) => {
    ctx.fillText(line, x + 96, y + index * 24, width - 116)
  })
}

function drawPill(ctx: CanvasRenderingContext2D, x: number, y: number, text: string, bg: string, color: string, minWidth = 300) {
  ctx.font = "700 20px PingFang SC, Microsoft YaHei, sans-serif"
  const pillWidth = Math.max(minWidth, ctx.measureText(text).width + 34)
  roundRect(ctx, x, y, pillWidth, 36, 18, bg)
  ctx.fillStyle = color
  ctx.fillText(text, x + 17, y + 24)
}

function preferenceText(period: keyof typeof periodLabels, pref: WeekPlanPreferences[typeof period]) {
  return `${periodLabels[period]} · ${profileLabels[pref.profile]} · 午 ${quotaText(pref.lunch)} · 晚 ${quotaText(pref.dinner)}`
}

function quotaText(quota: { meat_count: number; veg_count: number; soup_count: number }) {
  const total = quota.meat_count + quota.veg_count + quota.soup_count
  if (total === 0) return "跳过"
  return `${quota.meat_count}荤${quota.veg_count}素${quota.soup_count}汤`
}

function splitNames(names: string[]) {
  if (names.length === 0) return []
  const lines: string[] = []
  let current = ""
  names.forEach((name) => {
    const next = current ? `${current} · ${name}` : name
    if (next.length > 22 && current) {
      lines.push(current)
      current = name
    } else {
      current = next
    }
  })
  if (current) lines.push(current)
  return lines
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

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number, fill: string) {
  ctx.beginPath()
  ctx.roundRect(x, y, width, height, radius)
  ctx.fillStyle = fill
  ctx.fill()
}

function strokeRoundRect(ctx: CanvasRenderingContext2D, x: number, y: number, width: number, height: number, radius: number) {
  ctx.beginPath()
  ctx.roundRect(x, y, width, height, radius)
  ctx.stroke()
}
