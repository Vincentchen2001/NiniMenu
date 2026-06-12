import { useState, useMemo } from "react"
import { createPortal } from "react-dom"
import { useNavigate } from "react-router-dom"
import { useQuery, useMutation, useQueryClient } from "@tanstack/react-query"
import { recordsApi, statsApi, dayRatingApi, uploadApi, getUploadErrorMessage, weekPlanApi } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import type { MealRecord, DayRating } from "@/types"
import StatCard from "@/components/StatCard"
import SectionHeader from "@/components/SectionHeader"
import PageHeader from "@/components/PageHeader"
import PhotoViewer from "@/components/PhotoViewer"
import SwipeDeleteRow from "@/components/SwipeDeleteRow"
import { cardShadow } from "@/components/Card"
import toast from "react-hot-toast"
import { CalendarDays } from "lucide-react"

const weekdays = ["日", "一", "二", "三", "四", "五", "六"]

const moodOptions = [
  { key: "yum", emoji: "😋", label: "好吃" },
  { key: "ok", emoji: "😐", label: "一般" },
  { key: "no", emoji: "😵", label: "不想再吃" },
]

const mealMoodOptions = [
  { key: "great", emoji: "😋", label: "超满足" },
  { key: "ok", emoji: "😐", label: "还行吧" },
  { key: "meh", emoji: "😕", label: "不太行" },
]

const moodEmojiMap: Record<string, string> = { yum: "😋", ok: "😐", no: "😵", great: "😋", meh: "😕" }
const homeMoodEmojiMap: Record<string, string> = { happy: "😊", tired: "😫", lazy: "😌", spicy: "🤤", healthy: "🌿" }
const homeMoodLabelMap: Record<string, string> = { happy: "开心", tired: "疲惫", lazy: "想偷懒", spicy: "想吃辣", healthy: "想养生" }

const todayStr = () => `${new Date().getFullYear()}-${String(new Date().getMonth()+1).padStart(2,"0")}-${String(new Date().getDate()).padStart(2,"0")}`

function SwipeRowBg({ children, onDelete }: { children: React.ReactNode; onDelete: () => void }) {
  return (
    <SwipeDeleteRow actionWidth={80} onDelete={onDelete} className="bg-red">
      {children}
    </SwipeDeleteRow>
  )
}

function SwipeRowCard({ children, onDelete }: { children: React.ReactNode; onDelete: () => void }) {
  return (
    <SwipeDeleteRow actionWidth={72} onDelete={onDelete} className={`rounded-2xl border border-border ${cardShadow}`}>
      {children}
    </SwipeDeleteRow>
  )
}

export default function History() {
  const navigate = useNavigate()
  const qc = useQueryClient()
  const isAdminLoggedIn = useAuthStore((s) => s.isLoggedIn)
  const [calYear, setCalYear] = useState(new Date().getFullYear())
  const [calMonth, setCalMonth] = useState(new Date().getMonth())
  const [selectedDay, setSelectedDay] = useState<number | null>(new Date().getDate())
  const [ratingRecord, setRatingRecord] = useState<MealRecord | null>(null)
  const [ratingMood, setRatingMood] = useState<string>("")
  const [ratingRemark, setRatingRemark] = useState("")
  const [ratingPhoto, setRatingPhoto] = useState("")
  const [showMealRating, setShowMealRating] = useState(false)
  const [mealRatingMood, setMealRatingMood] = useState<string>("")
  const [mealRatingRemark, setMealRatingRemark] = useState("")
  const [mealPhotos, setMealPhotos] = useState<string[]>([])
  const [viewerPhotos, setViewerPhotos] = useState<string[]>([])
  const [viewerIdx, setViewerIdx] = useState(0)
  const [showViewer, setShowViewer] = useState(false)

  const deleteMut = useMutation({
    mutationFn: (id: number) => recordsApi.delete(id),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: ["records"] })
      const prev = qc.getQueryData(["records"])
      qc.setQueryData(["records"], (old: unknown) => {
        if (!old || typeof old !== "object" || old === null) return old
        const o = old as { items?: MealRecord[] }
        if (!o.items) return old
        return { ...o, items: o.items.filter((r) => r.id !== id) }
      })
      return { prev }
    },
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["records"] })
      qc.invalidateQueries({ queryKey: ["stats"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success("已删除")
    },
    onError: (_err, _id, context) => {
      if (context?.prev) qc.setQueryData(["records"], context.prev)
      toast.error("删除失败")
    },
  })

  const updateMut = useMutation({
    mutationFn: ({ id, data }: { id: number; data: Partial<MealRecord> }) => recordsApi.update(id, data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["records"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success("评价已保存")
      setRatingRecord(null)
    },
    onError: () => {
      toast.error("评价失败")
    },
  })

  const dayRatingMut = useMutation({
    mutationFn: (data: { meal_date: string; mood: string; remark?: string; photos?: string }) =>
      dayRatingApi.create(data),
    onSuccess: () => {
      qc.invalidateQueries({ queryKey: ["day-rating"] })
      qc.invalidateQueries({ queryKey: ["day-ratings"] })
      qc.invalidateQueries({ queryKey: ["photo-wall"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success("今日评价已保存")
      setShowMealRating(false)
      setMealPhotos([])
    },
    onError: () => {
      toast.error("评价保存失败")
    },
  })

  const updateDayPhotosMut = useMutation({
    mutationFn: (data: { meal_date: string; photos: string }) =>
      dayRatingApi.create(data),
    onMutate: async (data) => {
      const key = ["day-rating", data.meal_date]
      await qc.cancelQueries({ queryKey: key })
      const prev = qc.getQueryData(key)
      qc.setQueryData(key, (old: unknown) => {
        if (!old || typeof old !== "object" || old === null) return old
        const r = old as DayRating
        return { ...r, photos: data.photos }
      })
      return { prev, key }
    },
    onSuccess: (_, data) => {
      qc.invalidateQueries({ queryKey: ["day-rating", data.meal_date] })
      qc.invalidateQueries({ queryKey: ["day-ratings"] })
      qc.invalidateQueries({ queryKey: ["photo-wall"] })
      qc.invalidateQueries({ queryKey: ["achievements"] })
      toast.success("照片已删除")
    },
    onError: (_err, _data, context) => {
      if (context?.prev) qc.setQueryData(context.key, context.prev)
      toast.error("照片删除失败")
    },
  })

  const { data: stats } = useQuery({ queryKey: ["stats"], queryFn: () => statsApi.get() })
  const { data: recordsData } = useQuery({
    queryKey: ["records"],
    queryFn: () => recordsApi.list({ pageSize: "100" }),
  })

  const selectedDateKey = selectedDay !== null ? `${calYear}-${String(calMonth + 1).padStart(2, "0")}-${String(selectedDay).padStart(2, "0")}` : ""
  const monthStartKey = `${calYear}-${String(calMonth + 1).padStart(2, "0")}-01`
  const monthEndKey = `${calYear}-${String(calMonth + 1).padStart(2, "0")}-${String(new Date(calYear, calMonth + 1, 0).getDate()).padStart(2, "0")}`

  const { data: dayRatingData } = useQuery({
    queryKey: ["day-rating", selectedDateKey],
    queryFn: () => dayRatingApi.get(selectedDateKey),
    enabled: !!selectedDateKey,
  })
  const dayRating = (dayRatingData as DayRating | null) || null

  const { data: dayRatings = [] } = useQuery({
    queryKey: ["day-ratings", calYear, calMonth],
    queryFn: () => dayRatingApi.list({ date_from: monthStartKey, date_to: monthEndKey }),
  })

  const planMonthKey = `${calYear}-${String(calMonth + 1).padStart(2, "0")}`
  const { data: plannedByDate = {} } = useQuery({
    queryKey: ["week-plan-history", planMonthKey],
    queryFn: () => weekPlanApi.history(planMonthKey),
  })

  const records = useMemo(() => recordsData?.items ?? [], [recordsData?.items])

  const recordByDate = useMemo(() => {
    const m = new Map<string, MealRecord[]>()
    records.forEach((r) => {
      const arr = m.get(r.meal_date) || []
      arr.push(r)
      m.set(r.meal_date, arr)
    })
    return m
  }, [records])

  const dayRatingByDate = useMemo(() => {
    const m = new Map<string, DayRating>()
    dayRatings.forEach((r) => m.set(r.meal_date, r))
    return m
  }, [dayRatings])

  function openRating(r: MealRecord) {
    setRatingRecord(r)
    setRatingMood(r.mood || "")
    setRatingRemark(r.remark || "")
    setRatingPhoto(r.photo || "")
  }

  function submitRating() {
    if (!ratingRecord) return
    updateMut.mutate({
      id: ratingRecord.id,
      data: { mood: ratingMood, remark: ratingRemark, photo: ratingPhoto },
    })
  }

  async function handleRatingPhotoUpload() {
    if (!isAdminLoggedIn) {
      toast.error("上传照片需要先进入管理模式")
      navigate("/admin/login")
      return
    }
    const input = document.createElement("input")
    input.type = "file"
    input.accept = "image/jpg,image/jpeg,image/png,image/webp"
    input.onchange = async () => {
      const file = input.files?.[0]
      if (!file) return
      try {
        const res = await uploadApi.image(file)
        const url = res.data.data.url
        if (url) setRatingPhoto(url)
        else toast.error("上传返回异常")
      } catch (err) {
        toast.error(getUploadErrorMessage(err))
      }
    }
    input.click()
  }

  function parsePhotos(photosStr: string): string[] {
    try {
      const arr = JSON.parse(photosStr)
      return Array.isArray(arr) ? arr : []
    } catch {
      return []
    }
  }

  function openMealRating() {
    setMealRatingMood(dayRating?.mood || "")
    setMealRatingRemark(dayRating?.remark || "")
    setMealPhotos(dayRating?.photos ? parsePhotos(dayRating.photos) : [])
    setShowMealRating(true)
  }

  function submitMealRating() {
    if (!mealRatingMood || !selectedDateKey) return
    dayRatingMut.mutate({
      meal_date: selectedDateKey,
      mood: mealRatingMood,
      remark: mealRatingRemark,
      photos: JSON.stringify(mealPhotos),
    })
  }

  async function handlePhotoUpload() {
    if (!isAdminLoggedIn) {
      toast.error("上传照片需要先进入管理模式")
      navigate("/admin/login")
      return
    }

    const input = document.createElement("input")
    input.type = "file"
    input.multiple = true
    input.accept = "image/jpg,image/jpeg,image/png,image/webp"
    input.onchange = async () => {
      const files = Array.from(input.files || [])
      if (files.length === 0) return
      const remaining = 9 - mealPhotos.length
      if (remaining <= 0) {
        toast.error("最多上传9张照片")
        return
      }
      const toUpload = files.slice(0, remaining)
      if (files.length > remaining) {
        toast.error(`已选${files.length}张，仅上传前${remaining}张`)
      }
      let failed = 0
      let lastErrMsg = ""
      for (const file of toUpload) {
        try {
          const res = await uploadApi.image(file)
          const url = res.data.data.url
          if (url) {
            setMealPhotos((prev) => [...prev, url])
          } else {
            failed++
          }
        } catch (err) {
          failed++
          lastErrMsg = getUploadErrorMessage(err)
        }
      }
      if (failed > 0) toast.error(lastErrMsg || `${failed}张上传失败`)
    }
    input.click()
  }

  function removeMealPhoto(idx: number) {
    setMealPhotos((prev) => prev.filter((_, i) => i !== idx))
  }

  function removeDayPhoto(idx: number) {
    if (!isAdminLoggedIn) {
      toast.error("删除照片需要先进入管理模式")
      navigate("/admin/login")
      return
    }
    const dayPhotos = dayRating?.photos ? parsePhotos(dayRating.photos) : []
    const updated = dayPhotos.filter((_, i) => i !== idx)
    updateDayPhotosMut.mutate({
      meal_date: selectedDateKey,
      photos: JSON.stringify(updated),
    })
  }

  function openViewer(photos: string[], idx: number) {
    setViewerPhotos(photos)
    setViewerIdx(idx)
    setShowViewer(true)
  }

  function closeViewer() {
    setShowViewer(false)
  }

  function buildCalendar() {
    const firstDay = new Date(calYear, calMonth, 1).getDay()
    const daysInMonth = new Date(calYear, calMonth + 1, 0).getDate()
    const prevDays = new Date(calYear, calMonth, 0).getDate()
    const today = new Date()
    const cells: { day: number; isCurrentMonth: boolean; isToday: boolean; dateKey: string }[] = []

    for (let i = firstDay - 1; i >= 0; i--) {
      const d = prevDays - i
      const pm = calMonth === 0 ? 11 : calMonth - 1
      const py = calMonth === 0 ? calYear - 1 : calYear
      cells.push({ day: d, isCurrentMonth: false, isToday: false, dateKey: `${py}-${String(pm + 1).padStart(2, "0")}-${String(d).padStart(2, "0")}` })
    }
    for (let d = 1; d <= daysInMonth; d++) {
      const isToday = calYear === today.getFullYear() && calMonth === today.getMonth() && d === today.getDate()
      cells.push({ day: d, isCurrentMonth: true, isToday, dateKey: `${calYear}-${String(calMonth + 1).padStart(2, "0")}-${String(d).padStart(2, "0")}` })
    }
    const remaining = (7 - (firstDay + daysInMonth) % 7) % 7
    for (let i = 1; i <= remaining; i++) {
      const nm = calMonth === 11 ? 0 : calMonth + 1
      const ny = calMonth === 11 ? calYear + 1 : calYear
      cells.push({ day: i, isCurrentMonth: false, isToday: false, dateKey: `${ny}-${String(nm + 1).padStart(2, "0")}-${String(i).padStart(2, "0")}` })
    }
    return cells
  }

  const calCells = buildCalendar()
  const selectedRecords = selectedDay !== null ? (recordByDate.get(selectedDateKey) || []) : []

  function changeMonth(dir: number) {
    let m = calMonth + dir
    let y = calYear
    if (m > 11) { m = 0; y++ }
    if (m < 0) { m = 11; y-- }
    setCalMonth(m)
    setCalYear(y)
    setSelectedDay(null)
  }

  const topDish = stats?.top_dishes?.[0]

  return (
    <div className="animate-fadeUp">
      <PageHeader title="记录" subtitle="查看日历、评价和最近吃过的菜" icon={CalendarDays} />

      <div className="px-5 py-4 max-w-[640px] mx-auto">
        

        <div className={`bg-card rounded-2xl p-3.5 mb-5 ${cardShadow} border border-border`}>
          <div className="flex items-center justify-between mb-2.5">
            <div className="text-base font-bold">{calYear} 年 {calMonth + 1} 月</div>
            <div className="flex gap-2">
              <button onClick={() => changeMonth(-1)} className="w-7 h-7 rounded-full bg-bg flex items-center justify-center text-sm transition-all active:bg-primary-light">←</button>
              <button onClick={() => changeMonth(1)} className="w-7 h-7 rounded-full bg-bg flex items-center justify-center text-sm transition-all active:bg-primary-light">→</button>
            </div>
          </div>
          <div className="grid grid-cols-7 gap-1 text-center">
            {weekdays.map((w) => <div key={w} className="text-[10px] text-text3 font-semibold py-0.5">{w}</div>)}
            {calCells.map((cell, i) => {
              const hasRecord = recordByDate.has(cell.dateKey)
              const recs = recordByDate.get(cell.dateKey) || []
              const hasLunch = recs.some((r) => r.meal_type === "lunch")
              const hasDinner = recs.some((r) => r.meal_type === "dinner")
              const isSelected = cell.isCurrentMonth && selectedDay === cell.day
              const homeMood = cell.isCurrentMonth ? dayRatingByDate.get(cell.dateKey)?.home_mood : ""
              const homeMoodEmoji = homeMood ? homeMoodEmojiMap[homeMood] : ""
              return (
                <button
                  key={i}
                  onClick={() => cell.isCurrentMonth && setSelectedDay(cell.day)}
                  className={`h-7 flex items-center justify-center rounded-full text-[12px] cursor-pointer transition-all relative ${!cell.isCurrentMonth ? "text-text4" : ""} ${cell.isToday ? "font-bold text-primary" : ""} ${hasRecord ? "after:content-[''] after:absolute after:bottom-[2px] after:w-1 after:h-1 after:rounded-full " + (hasLunch && hasDinner ? "after:bg-gradient-to-r after:from-primary after:to-mint after:w-2 after:h-1 after:rounded-sm" : hasLunch ? "after:bg-primary" : "after:bg-mint") : ""} ${isSelected ? "bg-primary text-white font-bold after:bg-white" : "active:bg-primary-light"}`}
                >
                  {cell.day}
                  {homeMoodEmoji && <span className="absolute -top-0.5 right-0 text-[10px] leading-none">{homeMoodEmoji}</span>}
                </button>
              )
            })}
          </div>
        </div>

        {selectedDay !== null && (
          <div className={`bg-card rounded-2xl overflow-hidden mb-5 ${cardShadow} border border-border animate-pop-soft`}>
            <div className="px-4 py-3 bg-primary-light text-sm font-semibold text-primary flex items-center justify-between">
              <div className="flex items-center gap-1.5 flex-wrap">
                {calMonth + 1}月{selectedDay}日
                {selectedDateKey === todayStr() && <span className="text-[10px] font-medium bg-primary text-white px-1.5 py-px rounded-full">今天</span>}
                {dayRating?.home_mood && (
                  <span className="text-[10px] font-medium bg-white/80 text-primary px-1.5 py-px rounded-full">
                    {homeMoodEmojiMap[dayRating.home_mood] || "😃"} {homeMoodLabelMap[dayRating.home_mood] || "心情"}
                  </span>
                )}
              </div>
              <button
                onClick={openMealRating}
                className={`px-2.5 py-0.5 rounded-full text-[11px] font-semibold transition-all active:scale-95 ${dayRating?.mood ? "bg-mint text-white" : "bg-primary text-white"}`}
              >
                {dayRating?.mood ? "✓ 已评价" : "😌 评价"}
              </button>
            </div>
            {selectedDateKey && (plannedByDate[selectedDateKey]?.length ?? 0) > 0 && (
              <div className="px-4 py-2.5 border-b border-border">
                <div className="text-[11px] text-text3 mb-1.5">当天排的菜单</div>
                <div className="flex flex-wrap gap-1.5">
                  {plannedByDate[selectedDateKey].map((entry, i) => (
                    <span key={`${entry.dish_id}-${i}`} className="text-[11px] font-medium bg-primary-light text-primary px-2 py-0.5 rounded-full">
                      {entry.meal_type === "lunch" ? "午" : "晚"} · {entry.dish_name}
                    </span>
                  ))}
                </div>
              </div>
            )}
            {selectedRecords.length === 0 ? (
              <div className="py-5 px-4 text-center text-[13px] text-text3">{selectedDateKey === todayStr() ? "今天还没有记录哦~" : "这天没有记录"}</div>
            ) : (
              selectedRecords.map((r) => (
                <SwipeRowBg key={r.id} onDelete={() => deleteMut.mutate(r.id)}>
                  <div className="flex items-center gap-3 px-4 py-3 border-b border-border last:border-0 transition-all active:bg-bg w-full text-left cursor-pointer">
                    <span onClick={() => navigate(`/dishes/${r.dish_id}`)} className={`w-10 h-10 rounded-[10px] flex items-center justify-center text-xl flex-shrink-0 overflow-hidden ${r.meal_type === "lunch" ? "bg-primary-light" : "bg-mint-light"}`}>
                      {r.dish_image_url ? <img src={r.dish_image_url} alt={r.dish_name} className="w-full h-full object-cover" /> : r.dish_emoji}
                    </span>
                    <div className="flex-1 min-w-0" onClick={() => navigate(`/dishes/${r.dish_id}`)}>
                      <div className="text-sm font-semibold mb-px">{r.dish_name}</div>
                      <div className="text-[11px] text-text2">{r.meal_type === "lunch" ? "午餐" : "晚餐"}</div>
                    </div>
                    {r.mood ? (
                      <div onClick={(e) => { e.stopPropagation(); openRating(r) }} className="flex-shrink-0 flex items-center gap-1 cursor-pointer active:scale-95 transition-transform">
                        <span className="text-lg">{moodEmojiMap[r.mood]}</span>
                        {r.remark && <span className="text-[11px] text-text2 max-w-[60px] truncate">{r.remark}</span>}
                      </div>
                    ) : (
                      <button onClick={(e) => { e.stopPropagation(); openRating(r) }} className="flex-shrink-0 px-2 py-1 rounded-full text-[10px] font-medium bg-primary-light text-primary active:scale-95 transition-all">评价</button>
                    )}
                  </div>
                </SwipeRowBg>
              ))
            )}
            {dayRating?.mood && (
              <div className="px-4 py-2.5 bg-bg border-t border-border flex items-center gap-2.5 text-[13px]">
                <span className="text-xl">🍚</span>
                <span className="flex-1 text-text2">
                  {selectedRecords.length > 1 ? "午餐+晚餐" : "一餐"} · 整体评价
                  {dayRating.remark && <span className="ml-1 text-text"> {dayRating.remark}</span>}
                </span>
                <span className="text-xl">{moodEmojiMap[dayRating.mood] || "😋"}</span>
              </div>
            )}
            {(() => {
              const dayPhotos = dayRating?.photos ? parsePhotos(dayRating.photos) : []
              const hasPhotos = dayPhotos.length > 0
              if (!hasPhotos && !selectedDateKey) return null
              return (
                <div className="px-4 py-2.5 border-t border-border">
                  <div className="text-[11px] text-text3 mb-1.5">{hasPhotos ? `📷 照片 (${dayPhotos.length})` : "📷 拍张照记录一下？"}</div>
                  <div className="flex gap-1.5 overflow-x-auto scrollbar-none pb-1">
                    {dayPhotos.map((p, i) => (
                       <div
                         key={i}
                         className="w-14 h-14 rounded-md overflow-hidden flex-shrink-0 bg-bg border border-border flex items-center justify-center cursor-pointer transition-all active:scale-95 relative"
                         onClick={() => openViewer(dayPhotos, i)}
                       >
                          <img src={p} alt="" className="w-full h-full object-cover" />
                          {isAdminLoggedIn && (
                            <button
                             onClick={(e) => { e.stopPropagation(); removeDayPhoto(i) }}
                             className="absolute top-0 right-0 w-[16px] h-[16px] rounded-bl-md bg-black/50 text-white text-[9px] flex items-center justify-center active:bg-red"
                           >✕</button>
                         )}
                       </div>
                     ))}
                    <div
                      onClick={openMealRating}
                      className="w-14 h-14 rounded-md flex-shrink-0 border-2 border-dashed border-border2 flex flex-col items-center justify-center cursor-pointer transition-all active:border-primary active:text-primary text-text3"
                    >
                      <span className="text-base leading-none">+</span>
                      <span className="text-[9px]">添加</span>
                    </div>
                  </div>
                </div>
              )
            })()}
          </div>
        )}

        <div className="grid grid-cols-2 gap-3 mb-5">
          <StatCard value={stats?.total_records || 0} label="累计做饭" color="primary" />
          <StatCard value={topDish?.dish_name || "-"} label="最常吃的菜" color="mint" small />
          <StatCard value={stats?.lunch_count || 0} label="午餐次数" color="pink" />
          <StatCard value={stats?.dinner_count || 0} label="晚餐次数" color="yellow" />
        </div>

        <SectionHeader title="最近记录" />
        <div className="flex flex-col gap-2.5">
          {records.slice(0, 5).map((r) => (
            <SwipeRowCard key={r.id} onDelete={() => deleteMut.mutate(r.id)}>
              <div className="p-3.5 px-4 flex items-center gap-3.5 w-full text-left cursor-pointer transition-all active:bg-primary-light">
                <span onClick={() => navigate(`/dishes/${r.dish_id}`)} className={`w-12 h-12 rounded-[10px] flex items-center justify-center text-2xl flex-shrink-0 overflow-hidden ${r.meal_type === "lunch" ? "bg-primary-light" : "bg-mint-light"}`}>
                  {r.dish_image_url ? <img src={r.dish_image_url} alt={r.dish_name} className="w-full h-full object-cover" /> : r.dish_emoji}
                </span>
                <div className="flex-1 min-w-0" onClick={() => navigate(`/dishes/${r.dish_id}`)}>
                  <div className="text-sm font-semibold mb-px">{r.dish_name}</div>
                  <div className="text-xs text-text2">{r.meal_date} · {r.meal_type === "lunch" ? "午餐" : "晚餐"}</div>
                </div>
                {r.mood ? (
                  <div onClick={(e) => { e.stopPropagation(); openRating(r) }} className="flex-shrink-0 flex items-center gap-1 cursor-pointer active:scale-95 transition-transform">
                    <span className="text-lg">{moodEmojiMap[r.mood]}</span>
                    {r.remark && <span className="text-[11px] text-text2 max-w-[60px] truncate">{r.remark}</span>}
                  </div>
                ) : (
                  <button onClick={(e) => { e.stopPropagation(); openRating(r) }} className="flex-shrink-0 px-2 py-1 rounded-full text-[10px] font-medium bg-primary-light text-primary active:scale-95 transition-all">评价</button>
                )}
              </div>
            </SwipeRowCard>
          ))}
          {records.length === 0 && (
            <div className="text-center py-12">
              <span className="text-[56px] block mb-4 animate-float">📅</span>
              <div className="text-base font-semibold mb-1.5">还没有记录</div>
              <div className="text-[13px] text-text2">去首页推荐一道菜吧~</div>
            </div>
          )}
        </div>
      </div>

      {ratingRecord && createPortal(
        <div className="fixed inset-0 z-[400] bg-black/45 backdrop-blur-sm flex items-end justify-center" onClick={() => setRatingRecord(null)}>
          <div className="bg-card w-full max-w-[640px] rounded-t-2xl animate-slide-in-up" onClick={(e) => e.stopPropagation()}>
            <div className="w-9 h-1 rounded-full bg-border2 mx-auto mt-2" />
            <div className="px-5 py-4 border-b border-border flex items-center justify-between">
              <div className="text-base font-bold">{ratingRecord.dish_name} — 这道菜怎么样？</div>
              <button onClick={() => setRatingRecord(null)} className="w-7 h-7 rounded-full flex items-center justify-center text-text3 hover:bg-bg text-sm">✕</button>
            </div>
            <div className="p-5">
              <div className="flex justify-center gap-4 mb-5">
                {moodOptions.map((m) => (
                  <button
                    key={m.key}
                    onClick={() => setRatingMood(m.key)}
                    className={`flex flex-col items-center gap-1.5 p-3 px-5 rounded-2xl border-2 transition-all active:scale-90 ${ratingMood === m.key ? "border-primary bg-primary-light" : "border-transparent bg-bg"}`}
                  >
                    <span className="text-3xl">{m.emoji}</span>
                    <span className={`text-xs font-medium ${ratingMood === m.key ? "text-primary" : "text-text2"}`}>{m.label}</span>
                  </button>
                ))}
              </div>
              <div className="mb-5">
                <div className="text-xs text-text2 font-semibold mb-2">备注（可选）</div>
                <textarea
                  value={ratingRemark}
                  onChange={(e) => setRatingRemark(e.target.value)}
                  placeholder="这道菜怎么样..."
                  rows={2}
                  className="w-full p-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm resize-none focus:border-primary focus:ring-2 focus:ring-primary/10 outline-none transition-all"
                />
              </div>
              <div className="mb-5">
                <div className="text-xs text-text2 font-semibold mb-2">📷 成品照（可选，限1张）</div>
                {ratingPhoto ? (
                  <div className="relative w-20 h-20 rounded-[10px] overflow-hidden border border-border">
                    <img src={ratingPhoto} alt="" className="w-full h-full object-cover" />
                    <button
                      onClick={() => setRatingPhoto("")}
                      className="absolute top-0.5 right-0.5 w-[18px] h-[18px] rounded-full bg-black/50 text-white text-[10px] flex items-center justify-center"
                    >✕</button>
                  </div>
                ) : (
                  <div
                    onClick={handleRatingPhotoUpload}
                    className={`w-20 h-20 rounded-[10px] border-2 border-dashed border-border2 flex flex-col items-center justify-center cursor-pointer transition-all active:border-primary active:text-primary ${isAdminLoggedIn ? "text-text3" : "text-text4 bg-bg/60"}`}
                  >
                    <span className="text-xl leading-none">+</span>
                    <span className="text-[9px]">添加</span>
                  </div>
                )}
              </div>
              <button
                onClick={submitRating}
                disabled={!ratingMood}
                className="w-full py-3 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96 disabled:opacity-40 disabled:active:scale-100"
              >保存评价</button>
            </div>
          </div>
        </div>,
        document.body
      )}

      {showMealRating && createPortal(
        <div className="fixed inset-0 z-[400] bg-black/45 backdrop-blur-sm flex items-end justify-center" onClick={() => setShowMealRating(false)}>
          <div className="bg-card w-full max-w-[640px] rounded-t-2xl animate-slide-in-up" onClick={(e) => e.stopPropagation()}>
            <div className="w-9 h-1 rounded-full bg-border2 mx-auto mt-2" />
            <div className="px-5 py-4 border-b border-border flex items-center justify-between">
              <div className="text-base font-bold">🍲 这顿饭怎么样？</div>
              <button onClick={() => setShowMealRating(false)} className="w-7 h-7 rounded-full flex items-center justify-center text-text3 hover:bg-bg text-sm">✕</button>
            </div>
            <div className="p-5">
              <div className="text-center mb-3 text-[13px] text-text2">评价 {selectedDateKey} 的整体用餐感受</div>
              <div className="flex justify-center gap-4 mb-5">
                {mealMoodOptions.map((m) => (
                  <button
                    key={m.key}
                    onClick={() => setMealRatingMood(m.key)}
                    className={`flex flex-col items-center gap-1.5 p-3 px-5 rounded-2xl border-2 transition-all active:scale-90 ${mealRatingMood === m.key ? "border-primary bg-primary-light" : "border-transparent bg-bg"}`}
                  >
                    <span className="text-3xl">{m.emoji}</span>
                    <span className={`text-xs font-medium ${mealRatingMood === m.key ? "text-primary" : "text-text2"}`}>{m.label}</span>
                  </button>
                ))}
              </div>
              <div className="mb-5">
                <div className="text-xs text-text2 font-semibold mb-2">备注（可选）</div>
                <textarea
                  value={mealRatingRemark}
                  onChange={(e) => setMealRatingRemark(e.target.value)}
                  placeholder="今天这顿饭的整体感受..."
                  rows={2}
                  className="w-full p-3 rounded-[10px] border-[1.5px] border-border bg-bg text-sm resize-none focus:border-primary focus:ring-2 focus:ring-primary/10 outline-none transition-all"
                />
              </div>
              <div className="mb-5">
                <div className="text-xs text-text2 font-semibold mb-2">📷 上传照片（可选，最多9张）</div>
                <div className="flex gap-2 items-start">
                  <div
                    onClick={handlePhotoUpload}
                    className={`w-16 h-16 rounded-[10px] border-2 border-dashed border-border2 flex flex-col items-center justify-center cursor-pointer transition-all active:border-primary active:text-primary flex-shrink-0 ${isAdminLoggedIn ? "text-text3" : "text-text4 bg-bg/60"}`}
                  >
                    <span className="text-xl leading-none">+</span>
                    <span className="text-[9px]">添加</span>
                  </div>
                  <div className="flex gap-2 flex-wrap">
                    {mealPhotos.map((p, i) => (
                       <div key={i} className="w-16 h-16 rounded-[10px] overflow-hidden relative bg-bg flex items-center justify-center border border-border">
                          <img src={p} alt="" className="w-full h-full object-cover" />
                          <button
                            onClick={(e) => { e.stopPropagation(); removeMealPhoto(i) }}
                            className="absolute top-0.5 right-0.5 w-[18px] h-[18px] rounded-full bg-black/50 text-white text-[10px] flex items-center justify-center active:bg-red"
                         >✕</button>
                       </div>
                     ))}
                  </div>
                </div>
              </div>
              <button
                onClick={submitMealRating}
                disabled={!mealRatingMood}
                className="w-full py-3 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96 disabled:opacity-40 disabled:active:scale-100"
              >保存评价</button>
            </div>
          </div>
        </div>,
        document.body
      )}

      {showViewer && viewerPhotos.length > 0 && createPortal(
        <PhotoViewer photos={viewerPhotos} idx={viewerIdx} setIdx={setViewerIdx} onClose={closeViewer} />,
        document.body
      )}
    </div>
  )
}
