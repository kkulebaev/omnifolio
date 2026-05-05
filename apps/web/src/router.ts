import { createRouter, createWebHistory, type RouteRecordRaw } from "vue-router";
import { useAuthStore } from "@/stores/auth";

const routes: RouteRecordRaw[] = [
  {
    path: "/login",
    name: "login",
    component: () => import("@/features/auth/pages/LoginPage.vue"),
    meta: { layout: "auth", requiresGuest: true },
  },
  {
    path: "/",
    name: "dashboard",
    component: () => import("@/features/dashboard/pages/DashboardPage.vue"),
    meta: { layout: "app", requiresAuth: true },
  },
  {
    path: "/accounts",
    name: "accounts",
    component: () => import("@/features/account/pages/AccountListPage.vue"),
    meta: { layout: "app", requiresAuth: true },
  },
  {
    path: "/accounts/:id",
    name: "account-detail",
    component: () => import("@/features/account/pages/AccountDetailPage.vue"),
    meta: { layout: "app", requiresAuth: true },
  },
  {
    path: "/deposits",
    name: "deposits",
    component: () => import("@/features/deposits/pages/DepositListPage.vue"),
    meta: { layout: "app", requiresAuth: true },
  },
  {
    path: "/instruments",
    name: "instruments",
    component: () => import("@/features/instrument/pages/InstrumentListPage.vue"),
    meta: { layout: "app", requiresAuth: true },
  },
  {
    path: "/settings",
    name: "settings",
    component: () => import("@/features/settings/pages/SettingsPage.vue"),
    meta: { layout: "app", requiresAuth: true },
  },
];

export const router = createRouter({
  history: createWebHistory(),
  routes,
});

router.beforeEach(async (to) => {
  const auth = useAuthStore();
  await auth.readyPromise;
  if (to.meta.requiresAuth && !auth.isAuthenticated) {
    return { name: "login", query: { redirect: to.fullPath } };
  }
  if (to.meta.requiresGuest && auth.isAuthenticated) {
    return { name: "dashboard" };
  }
});
