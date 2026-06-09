import { BrowserRouter, Routes, Route } from "react-router-dom"
import { QueryClient, QueryClientProvider } from "@tanstack/react-query"
import { Toaster } from "react-hot-toast"
import { useEffect } from "react"
import { useAppInfoStore } from "@/store/useAppInfoStore"
import AppGate from "@/components/AppGate"
import MainLayout from "@/layouts/MainLayout"
import AdminLayout from "@/layouts/AdminLayout"
import SubPageLayout from "@/layouts/SubPageLayout"
import Home from "@/pages/Home"
import DishList from "@/pages/DishList"
import DishDetail from "@/pages/DishDetail"
import History from "@/pages/History"
import Favorites from "@/pages/Favorites"
import PhotoWall from "@/pages/PhotoWall"
import More from "@/pages/More"
import WeekPlan from "@/pages/WeekPlan"
import Tomorrow from "@/pages/Tomorrow"
import Achievements from "@/pages/Achievements"
import CookMode from "@/pages/CookMode"
import AdminLogin from "@/pages/admin/Login"
import AdminDashboard from "@/pages/admin/Dashboard"
import AdminDishes from "@/pages/admin/Dishes"
import AdminDishEdit from "@/pages/admin/DishEdit"
import AdminSettings from "@/pages/admin/Settings"
import AdminAchievements from "@/pages/admin/Achievements"
import AdminQuotes from "@/pages/admin/Quotes"
import AdminRecords from "@/pages/admin/Records"

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      staleTime: 30000,
      retry: 1,
    },
  },
})

export default function App() {
  const fetchAppInfo = useAppInfoStore((s) => s.fetch)
  const appName = useAppInfoStore((s) => s.appName)

  useEffect(() => { fetchAppInfo() }, [fetchAppInfo])
  useEffect(() => { document.title = appName }, [appName])

  return (
    <QueryClientProvider client={queryClient}>
      <BrowserRouter>
        <Routes>
          <Route element={<AppGate><MainLayout /></AppGate>}>
            <Route path="/" element={<Home />} />
            <Route path="/dishes" element={<DishList />} />
            <Route path="/history" element={<History />} />
            <Route path="/week-plan" element={<WeekPlan />} />
            <Route path="/favorites" element={<Favorites />} />
            <Route path="/more" element={<More />} />
          </Route>
          <Route element={<AppGate><SubPageLayout /></AppGate>}>
            <Route path="/dishes/:id" element={<DishDetail />} />
            <Route path="/photo-wall" element={<PhotoWall />} />
            <Route path="/tomorrow" element={<Tomorrow />} />
            <Route path="/achievements" element={<Achievements />} />
          </Route>
          <Route path="/dishes/:id/cook" element={<AppGate><CookMode /></AppGate>} />
          <Route path="/admin" element={<AdminLayout />}>
            <Route path="login" element={<AdminLogin />} />
            <Route path="dashboard" element={<AdminDashboard />} />
            <Route path="dishes" element={<AdminDishes />} />
            <Route path="dishes/new" element={<AdminDishEdit />} />
            <Route path="dishes/:id" element={<AdminDishEdit />} />
          <Route path="settings" element={<AdminSettings />} />
          <Route path="achievements" element={<AdminAchievements />} />
          <Route path="quotes" element={<AdminQuotes />} />
          <Route path="records" element={<AdminRecords />} />
          </Route>
        </Routes>
      </BrowserRouter>
      <Toaster
        position="top-center"
        toastOptions={{
          duration: 2000,
          style: {
            borderRadius: "9999px",
            background: "#FFFFFF",
            color: "#1A1A2E",
            fontSize: "14px",
            padding: "10px 24px",
            boxShadow: "0 8px 32px rgba(0,0,0,.1)",
          },
        }}
      />
    </QueryClientProvider>
  )
}
