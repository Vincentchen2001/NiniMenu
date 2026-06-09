import { useState, useEffect } from "react"
import { useLocation, useNavigate } from "react-router-dom"
import { useMutation } from "@tanstack/react-query"
import { authApi } from "@/api"
import { useAuthStore } from "@/store/useAuthStore"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import toast from "react-hot-toast"

export default function AdminLogin() {
  const navigate = useNavigate()
  const location = useLocation()
  const { isLoggedIn, login } = useAuthStore()
  const appName = useAppInfoStore((s) => s.appName)
  const [password, setPassword] = useState("")
  const searchParams = new URLSearchParams(location.search)
  const redirectParam = searchParams.get("redirect") || ""
  const redirectTo = redirectParam.startsWith("/admin/") && !redirectParam.startsWith("/admin/login")
    ? redirectParam
    : "/admin/dashboard"

  useEffect(() => {
    if (isLoggedIn) navigate(redirectTo, { replace: true })
  }, [isLoggedIn, navigate, redirectTo])

  const loginMut = useMutation({
    mutationFn: () => authApi.login(password),
    onSuccess: (data) => {
      login(data.token)
      toast.success("✅ 登录成功")
      navigate(redirectTo, { replace: true })
    },
    onError: () => toast.error("密码错误"),
  })

  return (
    <div className="min-h-screen bg-bg flex items-center justify-center px-5">
      <div className="w-full max-w-[360px]">
        <h3 className="text-xl font-bold mb-1 text-center">🔒 {appName} 管理模式</h3>
        <p className="text-[13px] text-text2 text-center mb-5">输入管理密码进入菜品管理</p>
        <div className="mb-4">
          <label className="block text-[13px] font-semibold mb-1.5 text-text2">管理密码</label>
          <input
            type="password"
            placeholder="请输入密码"
            value={password}
            onChange={(e) => setPassword(e.target.value)}
            onKeyDown={(e) => e.key === "Enter" && loginMut.mutate()}
            className="w-full py-2.5 px-3.5 rounded-[10px] border-[1.5px] border-border bg-bg text-sm transition-all focus:border-primary focus:shadow-[0_0_0_3px_rgba(232,115,74,.1)] outline-none"
          />
        </div>
        <button onClick={() => loginMut.mutate()} disabled={loginMut.isPending} className="w-full py-2.5 px-5 rounded-full text-sm font-semibold bg-primary text-white transition-all active:scale-96 hover:bg-primary-dark disabled:opacity-50">进入管理</button>
        <button onClick={() => navigate("/")} className="w-full py-2.5 px-5 rounded-full text-sm font-medium text-text2 mt-2">取消</button>
      </div>
    </div>
  )
}
