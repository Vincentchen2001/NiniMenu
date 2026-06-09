import { Outlet, useNavigate, useLocation } from "react-router-dom"
import { useEffect } from "react"
import { useAuthStore } from "@/store/useAuthStore"
import { useAppInfoStore } from "@/store/useAppInfoStore"

const navItems = [
  { path: "/admin/dashboard", icon: "📊", label: "概览" },
  { path: "/admin/dishes", icon: "🍳", label: "菜品" },
  { path: "/admin/records", icon: "📋", label: "记录" },
  { path: "/admin/quotes", icon: "💬", label: "语录" },
  { path: "/admin/achievements", icon: "🏆", label: "成就" },
  { path: "/admin/settings", icon: "⚙", label: "设置" },
]

export default function AdminLayout() {
  const navigate = useNavigate()
  const location = useLocation()
  const { isLoggedIn, logout } = useAuthStore()
  const appName = useAppInfoStore((s) => s.appName)
  const currentAdminTarget = `${location.pathname}${location.search}${location.hash}`

  useEffect(() => {
    if (location.pathname !== "/admin/login" && !isLoggedIn) {
      navigate(`/admin/login?redirect=${encodeURIComponent(currentAdminTarget)}`, { replace: true })
    }
  }, [currentAdminTarget, isLoggedIn, location.pathname, navigate])

  useEffect(() => {
    const onExpired = () => {
      logout()
      navigate(`/admin/login?redirect=${encodeURIComponent(currentAdminTarget)}`, { replace: true })
    }
    window.addEventListener("admin-auth-expired", onExpired)
    return () => window.removeEventListener("admin-auth-expired", onExpired)
  }, [currentAdminTarget, logout, navigate])

  const handleLogout = () => {
    logout()
    navigate("/")
  }

  const isActive = (path: string) => location.pathname === path || (path !== "/admin/dashboard" && location.pathname.startsWith(path))

  if (!isLoggedIn && location.pathname !== "/admin/login") {
    return (
      <div className="flex h-dvh items-center justify-center bg-bg px-5 text-sm font-semibold text-text3">
        正在进入管理登录...
      </div>
    )
  }

  return (
    <div className="h-dvh bg-bg flex flex-col overflow-hidden">
      {location.pathname !== "/admin/login" && (
        <>
          <header className="sticky top-0 z-10 bg-bg/88 backdrop-blur-xl px-5 py-3 flex items-center gap-3 border-b border-border flex-shrink-0">
            <button onClick={() => navigate("/")} className="w-9 h-9 rounded-full flex items-center justify-center text-lg bg-card shadow-sm">←</button>
            <div className="flex-1 font-bold text-base">{appName} 管理</div>
            <button onClick={handleLogout} className="w-9 h-9 rounded-full flex items-center justify-center text-lg">🔓</button>
          </header>
          <nav className="bg-bg/88 backdrop-blur-xl border-b border-border flex-shrink-0">
            <div className="flex px-3 py-2 gap-1 max-w-[640px] mx-auto overflow-x-auto overscroll-x-contain scrollbar-none" style={{ WebkitOverflowScrolling: "touch" }}>
              {navItems.map((item) => (
                <button
                  key={item.path}
                  onClick={() => navigate(item.path)}
                  className={`flex items-center gap-1.5 px-3.5 py-1.5 rounded-full text-xs font-semibold whitespace-nowrap flex-shrink-0 transition-all ${isActive(item.path) ? "bg-primary text-white" : "text-text2 hover:bg-card"}`}
                >
                  <span>{item.icon}</span>
                  <span>{item.label}</span>
                </button>
              ))}
            </div>
          </nav>
        </>
      )}
      <div className="flex-1 overflow-y-auto overscroll-y-contain">
        <Outlet />
      </div>
    </div>
  )
}
