import { NavLink, useLocation, useNavigate } from "react-router-dom"
import { useEffect, useRef, useState } from "react"
import { CalendarCheck, CalendarDays, Heart, Home as HomeIcon, Menu, UtensilsCrossed } from "lucide-react"
import { gsap, motionDuration, useGSAP } from "@/lib/gsap"
import Home from "@/pages/Home"
import DishList from "@/pages/DishList"
import History from "@/pages/History"
import WeekPlan from "@/pages/WeekPlan"
import Favorites from "@/pages/Favorites"
import More from "@/pages/More"

const tabs = [
  { path: "/", Component: Home, icon: HomeIcon, label: "首页", exact: true },
  { path: "/dishes", Component: DishList, icon: UtensilsCrossed, label: "菜品" },
  { path: "/history", Component: History, icon: CalendarDays, label: "记录" },
  { path: "/week-plan", Component: WeekPlan, icon: CalendarCheck, label: "菜单" },
  { path: "/favorites", Component: Favorites, icon: Heart, label: "收藏" },
  { path: "/more", Component: More, icon: Menu, label: "更多" },
]

const TAB_COUNT = tabs.length
const STEP_PCT = 100 / TAB_COUNT

const EDGE_ZONE = 36
const SWIPE_MIN = 30

export default function MainLayout() {
  const location = useLocation()
  const navigate = useNavigate()
  const [activeIdx, setActiveIdx] = useState(() =>
    tabs.findIndex((t) => t.path === location.pathname)
  )
  const stripRef = useRef<HTMLDivElement>(null)
  const didMountRef = useRef(false)
  const animatingRef = useRef(false)
  const offsetXRef = useRef(0)
  const stateRef = useRef({ activeIdx })
  stateRef.current = { activeIdx }

  const touchRef = useRef<{
    startX: number
    startY: number
    startEdge: "left" | "right"
    directionDecided: boolean
    isHorizontal: boolean | null
  } | null>(null)

  useEffect(() => {
    const idx = tabs.findIndex((t) => t.path === location.pathname)
    if (idx >= 0) setActiveIdx(idx)
  }, [location.pathname])

  useGSAP(() => {
    const el = stripRef.current
    if (!el) return
    const duration = didMountRef.current ? motionDuration(0.32) : 0
    didMountRef.current = true
    offsetXRef.current = 0
    animatingRef.current = duration > 0
    gsap.to(el, {
      xPercent: -activeIdx * STEP_PCT,
      x: 0,
      duration,
      ease: "power3.out",
      force3D: true,
      onComplete: () => {
        animatingRef.current = false
      },
    })
  }, { dependencies: [activeIdx], scope: stripRef })

  useEffect(() => {
    function onTouchStart(e: TouchEvent) {
      const s = stateRef.current
      if (animatingRef.current) return
      const t = e.touches[0]
      const x = t.clientX
      const vw = window.innerWidth
      let startEdge: "left" | "right" | null = null
      if (x < EDGE_ZONE) startEdge = "left"
      else if (x > vw - EDGE_ZONE) startEdge = "right"
      if (!startEdge) return
      if (startEdge === "left" && s.activeIdx === 0) return
      if (startEdge === "right" && s.activeIdx === tabs.length - 1) return
      touchRef.current = {
        startX: x,
        startY: t.clientY,
        startEdge,
        directionDecided: false,
        isHorizontal: null,
      }
    }

    function onTouchMove(e: TouchEvent) {
      const ref = touchRef.current
      if (!ref) {
        const t = e.touches[0]
        const x = t.clientX
        const vw = window.innerWidth
        if (x < EDGE_ZONE || x > vw - EDGE_ZONE) e.preventDefault()
        return
      }
      e.preventDefault()

      const dx = e.touches[0].clientX - ref.startX
      const dy = e.touches[0].clientY - ref.startY

      if (!ref.directionDecided) {
        if (Math.abs(dx) < 8 && Math.abs(dy) < 8) return
        ref.directionDecided = true
        ref.isHorizontal = Math.abs(dx) > Math.abs(dy)
      }

      if (!ref.isHorizontal) {
        touchRef.current = null
        offsetXRef.current = 0
        gsap.to(stripRef.current, { x: 0, duration: motionDuration(0.18), ease: "power2.out" })
        return
      }

      let clampedDx = dx
      if (ref.startEdge === "left") {
        if (dx < 0) {
          touchRef.current = null
          offsetXRef.current = 0
          gsap.to(stripRef.current, { x: 0, duration: motionDuration(0.18), ease: "power2.out" })
          return
        }
      } else {
        if (dx > 0) {
          touchRef.current = null
          offsetXRef.current = 0
          gsap.to(stripRef.current, { x: 0, duration: motionDuration(0.18), ease: "power2.out" })
          return
        }
      }

      const vw = window.innerWidth
      const maxDx = vw * 0.35
      clampedDx = Math.max(-maxDx, Math.min(maxDx, clampedDx))

      const rubber = clampedDx * (1 - Math.abs(clampedDx) / (vw * 1.2))
      offsetXRef.current = rubber
      gsap.set(stripRef.current, { x: rubber, force3D: true })
    }

    function onTouchEnd() {
      const ref = touchRef.current
      if (!ref) return
      const s = stateRef.current
      const dx = offsetXRef.current
      if (ref.startEdge === "left" && dx > SWIPE_MIN && s.activeIdx > 0) {
        offsetXRef.current = 0
        setActiveIdx(s.activeIdx - 1)
        navigate(tabs[s.activeIdx - 1].path)
      } else if (ref.startEdge === "right" && dx < -SWIPE_MIN && s.activeIdx < tabs.length - 1) {
        offsetXRef.current = 0
        setActiveIdx(s.activeIdx + 1)
        navigate(tabs[s.activeIdx + 1].path)
      } else {
        if (stripRef.current) {
          offsetXRef.current = 0
          animatingRef.current = true
          gsap.to(stripRef.current, {
            xPercent: -s.activeIdx * STEP_PCT,
            x: 0,
            duration: motionDuration(0.25),
            ease: "power3.out",
            onComplete: () => {
              animatingRef.current = false
            },
          })
        }
      }
      touchRef.current = null
    }

    document.addEventListener("touchstart", onTouchStart, { passive: true })
    document.addEventListener("touchmove", onTouchMove, { passive: false })
    document.addEventListener("touchend", onTouchEnd, { passive: true })
    return () => {
      document.removeEventListener("touchstart", onTouchStart)
      document.removeEventListener("touchmove", onTouchMove)
      document.removeEventListener("touchend", onTouchEnd)
    }
  }, [navigate])

  return (
    <div className="h-dvh bg-bg overflow-hidden" style={{ touchAction: "pan-y" }}>
      <div className="h-full overflow-hidden">
        <div
          ref={stripRef}
          className="flex h-full will-change-transform"
          style={{
            width: `${TAB_COUNT * 100}%`,
          }}
        >
          {tabs.map((tab) => (
            <div key={tab.path} className="h-full overflow-y-auto overscroll-y-contain pb-[calc(60px+env(safe-area-inset-bottom))]" style={{ width: `${STEP_PCT}%`, flexShrink: 0 }}>
              <tab.Component />
            </div>
          ))}
        </div>
      </div>

      <nav
        className="fixed bottom-0 left-0 right-0 z-[100] border-t border-white/70 bg-white/92 backdrop-blur-2xl shadow-[0_-10px_30px_rgba(26,26,46,.07)]"
        style={{ paddingBottom: "env(safe-area-inset-bottom)" }}
      >
        <div className="mx-auto grid h-[58px] max-w-[640px] grid-cols-6 items-center px-2">
          {tabs.map((tab) => {
            const Icon = tab.icon
            const isActive = activeIdx === tabs.indexOf(tab)
            return (
              <NavLink
                key={tab.path}
                to={tab.path}
                end={tab.exact}
                onClick={(e) => {
                  e.preventDefault()
                  const idx = tabs.indexOf(tab)
                  if (idx !== activeIdx) {
                    setActiveIdx(idx)
                    navigate(tab.path)
                  }
                }}
                className={() =>
                  `group relative flex h-12 min-w-0 flex-col items-center justify-center gap-0.5 rounded-[13px] text-[10px] font-semibold transition-all active:scale-95 ${isActive ? "text-primary" : "text-text3 hover:text-text2"}`
                }
              >
                {() => (
                  <>
                    <span className={`flex h-6 w-8 items-center justify-center rounded-full transition-all ${isActive ? "bg-primary-light text-primary shadow-[inset_0_0_0_1px_rgba(232,115,74,.10)]" : "text-text3 group-hover:bg-bg"}`}>
                      <Icon size={18} strokeWidth={isActive ? 2.6 : 2.1} />
                    </span>
                    <span className={`max-w-full truncate leading-none transition-colors ${isActive ? "text-primary" : "text-text3"}`}>
                      {tab.label}
                    </span>
                  </>
                )}
              </NavLink>
            )
          })}
        </div>
      </nav>
    </div>
  )
}
