import type { WeekPlan, Dish } from "@/types"
import { getDishImageUrl } from "@/components/DishImage"

// 导出「一周菜单」长图：面向老人、在微信里直接看图。
// 设计要点：单列竖向、大字、真实菜品图、午晚分段，去掉统计/配额等规划信息。

type MealKey = "lunch" | "dinner"
type MealBlock = { meal: MealKey; dishes: Dish[] }
type DayBlock = { day: WeekPlan["days"][number]; meals: MealBlock[]; height: number }

// ---- 主题（逻辑坐标，基准宽 430px ≈ 微信全屏看图宽度）----
const W = 430
const SCALE = 3 // 输出 1290px 宽，微信缩放后依旧清晰
const PAD_X = 22
const PAD_TOP = 34
const PAD_BOTTOM = 30

const C = {
  bg: "#F3E9DC",
  card: "#FFFDFA",
  ink: "#26233A",
  ink2: "#403C58",
  muted: "#A98F76",
  line: "rgba(180, 140, 100, 0.24)",
  coral: "#E8734A",
  coralSoft: "#FCE3D6",
  coralInk: "#BF4F2B",
  green: "#2C9E78",
  greenSoft: "#D6F0E4",
  greenInk: "#1C7B5B",
  ph1: "#F3D9C7",
  ph2: "#E7BFA6",
}

const mealMeta: Record<MealKey, { name: string; soft: string; ink: string }> = {
  lunch: { name: "午餐", soft: C.coralSoft, ink: C.coralInk },
  dinner: { name: "晚餐", soft: C.greenSoft, ink: C.greenInk },
}

const FONT = `"PingFang SC", "Hiragino Sans GB", "Microsoft YaHei", sans-serif`
const font = (size: number, weight = 400) => `${weight} ${size}px ${FONT}`

// ---- 布局尺寸 ----
const GRID_GAP = 12
const CARD_W = (W - PAD_X * 2 - GRID_GAP) / 2
const CARD_IMG_H = 104
const CARD_NAME_H = 44
const CARD_H = CARD_IMG_H + CARD_NAME_H
const CARD_RADIUS = 16

const HEADER_H = 84
const DAYHEAD_H = 40
const MEAL_LABEL_H = 30
const MEAL_GAP = 12
const DAY_GAP = 16

export async function exportWeekPlanAsPng(plan: WeekPlan): Promise<string | undefined> {
  const days = (plan.days || []).filter((d) => d.lunch.length + d.dinner.length > 0)
  if (days.length === 0) return undefined

  // 预加载所有菜品图（失败的走占位，避免污染画布或卡住导出）
  const dishes = days.flatMap((d) => [...d.lunch, ...d.dinner])
  const imgMap = await preloadImages(dishes)

  // 量算高度
  const blocks: DayBlock[] = days.map((day) => {
    const meals = buildMeals(day)
    return { day, meals, height: measureDay(meals) }
  })
  let totalH = PAD_TOP + HEADER_H
  blocks.forEach((b, i) => {
    totalH += b.height
    if (i < blocks.length - 1) totalH += DAY_GAP
  })
  totalH += PAD_BOTTOM

  // 画布
  const canvas = document.createElement("canvas")
  canvas.width = Math.round(W * SCALE)
  canvas.height = Math.round(totalH * SCALE)
  const ctx = canvas.getContext("2d")
  if (!ctx) return undefined
  ctx.scale(SCALE, SCALE)
  ctx.textBaseline = "alphabetic"

  // 背景
  ctx.fillStyle = C.bg
  ctx.fillRect(0, 0, W, totalH)

  // 标题
  let y = PAD_TOP
  ctx.textAlign = "center"
  ctx.fillStyle = C.ink
  ctx.font = font(36, 900)
  ctx.fillText("一周菜单", W / 2, y + 34)
  ctx.fillStyle = C.muted
  ctx.font = font(15, 600)
  ctx.fillText(dateRange(plan), W / 2, y + 60)
  ctx.textAlign = "left"
  y += HEADER_H

  // 每天
  blocks.forEach((block, i) => {
    drawDay(ctx, block, imgMap, y)
    y += block.height
    if (i < blocks.length - 1) y += DAY_GAP
  })

  const fileName = `一周菜单-${days[0].date}.png`
  const link = document.createElement("a")
  link.download = fileName
  link.href = canvas.toDataURL("image/png")
  link.click()
  return fileName
}

function buildMeals(day: WeekPlan["days"][number]): MealBlock[] {
  const meals: MealBlock[] = []
  if (day.lunch.length > 0) meals.push({ meal: "lunch", dishes: day.lunch })
  if (day.dinner.length > 0) meals.push({ meal: "dinner", dishes: day.dinner })
  return meals
}

function measureDay(meals: MealBlock[]): number {
  let h = DAYHEAD_H
  meals.forEach((m, i) => {
    const rows = Math.ceil(m.dishes.length / 2)
    h += MEAL_LABEL_H + rows * CARD_H + (rows - 1) * GRID_GAP
    if (i < meals.length - 1) h += MEAL_GAP
  })
  return h
}

function drawDay(
  ctx: CanvasRenderingContext2D,
  block: DayBlock,
  imgMap: Map<string, HTMLImageElement>,
  top: number,
) {
  const { day, meals } = block

  // 星期 + 日期 + 横线
  ctx.fillStyle = C.ink
  ctx.font = font(23, 900)
  ctx.fillText(day.day_name, PAD_X, top + 23)
  const wdW = ctx.measureText(day.day_name).width
  ctx.fillStyle = C.muted
  ctx.font = font(14, 600)
  ctx.fillText(formatDate(day.date), PAD_X + wdW + 10, top + 22)
  const dtW = ctx.measureText(formatDate(day.date)).width
  const lineX = PAD_X + wdW + 10 + dtW + 12
  ctx.strokeStyle = C.line
  ctx.lineWidth = 2
  ctx.beginPath()
  ctx.moveTo(lineX, top + 16)
  ctx.lineTo(W - PAD_X, top + 16)
  ctx.stroke()

  let y = top + DAYHEAD_H
  meals.forEach((m, i) => {
    drawMealLabel(ctx, PAD_X, y, m.meal)
    y += MEAL_LABEL_H
    m.dishes.forEach((dish, idx) => {
      const col = idx % 2
      const row = Math.floor(idx / 2)
      const cx = PAD_X + col * (CARD_W + GRID_GAP)
      const cy = y + row * (CARD_H + GRID_GAP)
      const url = getDishImageUrl(dish) || ""
      drawCard(ctx, cx, cy, dish, imgMap.get(url))
    })
    const rows = Math.ceil(m.dishes.length / 2)
    y += rows * CARD_H + (rows - 1) * GRID_GAP
    if (i < meals.length - 1) y += MEAL_GAP
  })
}

function drawMealLabel(ctx: CanvasRenderingContext2D, x: number, y: number, meal: MealKey) {
  const m = mealMeta[meal]
  ctx.font = font(13, 800)
  const tw = ctx.measureText(m.name).width
  const pillW = tw + 22
  const pillH = 24
  roundRect(ctx, x, y, pillW, pillH, 12)
  ctx.fillStyle = m.soft
  ctx.fill()
  ctx.fillStyle = m.ink
  ctx.textBaseline = "middle"
  ctx.fillText(m.name, x + 11, y + pillH / 2 + 1)
  ctx.textBaseline = "alphabetic"
}

function drawCard(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  dish: Dish,
  img: HTMLImageElement | undefined,
) {
  // 卡片底（带柔和阴影）
  ctx.save()
  ctx.shadowColor = "rgba(150, 100, 60, 0.16)"
  ctx.shadowBlur = 14
  ctx.shadowOffsetY = 6
  roundRect(ctx, x, y, CARD_W, CARD_H, CARD_RADIUS)
  ctx.fillStyle = C.card
  ctx.fill()
  ctx.restore()

  // 菜品图（cover 裁剪 + 顶部圆角）或占位
  if (img && img.naturalWidth > 0) {
    drawImageCover(ctx, img, x, y, CARD_W, CARD_IMG_H, CARD_RADIUS)
  } else {
    drawPlaceholder(ctx, x, y, CARD_W, CARD_IMG_H, CARD_RADIUS, dish.name)
  }

  // 菜名（居中，过长自动缩字号 / 省略）
  ctx.fillStyle = C.ink2
  ctx.textAlign = "center"
  let size = 18
  ctx.font = font(size, 700)
  while (ctx.measureText(dish.name).width > CARD_W - 16 && size > 13) {
    size -= 1
    ctx.font = font(size, 700)
  }
  ctx.fillText(ellipsize(ctx, dish.name, CARD_W - 16), x + CARD_W / 2, y + CARD_IMG_H + 28)
  ctx.textAlign = "left"
}

function drawImageCover(
  ctx: CanvasRenderingContext2D,
  img: HTMLImageElement,
  x: number,
  y: number,
  w: number,
  h: number,
  radius: number,
) {
  ctx.save()
  roundRectTop(ctx, x, y, w, h, radius)
  ctx.clip()
  const scale = Math.max(w / img.naturalWidth, h / img.naturalHeight)
  const dw = img.naturalWidth * scale
  const dh = img.naturalHeight * scale
  ctx.drawImage(img, x + (w - dw) / 2, y + (h - dh) / 2, dw, dh)
  ctx.restore()
}

function drawPlaceholder(
  ctx: CanvasRenderingContext2D,
  x: number,
  y: number,
  w: number,
  h: number,
  radius: number,
  name: string,
) {
  ctx.save()
  roundRectTop(ctx, x, y, w, h, radius)
  ctx.clip()
  const g = ctx.createLinearGradient(x, y, x + w, y + h)
  g.addColorStop(0, C.ph1)
  g.addColorStop(1, C.ph2)
  ctx.fillStyle = g
  ctx.fillRect(x, y, w, h)
  ctx.fillStyle = "rgba(255, 255, 255, 0.9)"
  ctx.font = font(34, 800)
  ctx.textAlign = "center"
  ctx.textBaseline = "middle"
  ctx.fillText(name.slice(0, 1), x + w / 2, y + h / 2)
  ctx.textAlign = "left"
  ctx.textBaseline = "alphabetic"
  ctx.restore()
}

async function preloadImages(dishes: Dish[]): Promise<Map<string, HTMLImageElement>> {
  const urls = Array.from(
    new Set(dishes.map((d) => getDishImageUrl(d)).filter((u): u is string => !!u)),
  )
  const map = new Map<string, HTMLImageElement>()
  await Promise.all(
    urls.map(
      (url) =>
        new Promise<void>((resolve) => {
          const img = new Image()
          img.crossOrigin = "anonymous"
          img.onload = () => {
            map.set(url, img)
            resolve()
          }
          img.onerror = () => resolve()
          img.src = url
        }),
    ),
  )
  return map
}

function formatDate(date: string): string {
  const parts = date.split("-")
  if (parts.length < 3) return date
  return `${Number(parts[1])}月${Number(parts[2])}日`
}

function dateRange(plan: WeekPlan): string {
  const days = plan.days || []
  if (days.length === 0) return ""
  const first = days[0]?.date
  const last = days[days.length - 1]?.date
  if (!first) return ""
  return last && last !== first ? `${formatDate(first)} — ${formatDate(last)}` : formatDate(first)
}

function ellipsize(ctx: CanvasRenderingContext2D, text: string, maxWidth: number): string {
  if (ctx.measureText(text).width <= maxWidth) return text
  let t = text
  while (t.length > 1 && ctx.measureText(`${t}…`).width > maxWidth) t = t.slice(0, -1)
  return `${t}…`
}

function roundRect(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath()
  ctx.roundRect(x, y, w, h, r)
}

function roundRectTop(ctx: CanvasRenderingContext2D, x: number, y: number, w: number, h: number, r: number) {
  ctx.beginPath()
  ctx.moveTo(x, y + h)
  ctx.lineTo(x, y + r)
  ctx.arcTo(x, y, x + r, y, r)
  ctx.lineTo(x + w - r, y)
  ctx.arcTo(x + w, y, x + w, y + r, r)
  ctx.lineTo(x + w, y + h)
  ctx.closePath()
}
