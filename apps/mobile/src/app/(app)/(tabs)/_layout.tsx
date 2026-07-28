import { Tabs } from "expo-router";

import { useAuth } from "@/auth/AuthProvider";
import { Icon, type IconName } from "@/components/ui/Icon";
import { colors, minimumTouchTarget, typography } from "@/theme/tokens";

const tabs: {
  name: string;
  title: string;
  icon: IconName;
}[] = [
  { name: "home", title: "Beranda", icon: "view-dashboard-outline" },
  { name: "sell", title: "Transaksi", icon: "receipt-text-plus-outline" },
  { name: "history", title: "Riwayat", icon: "history" },
  { name: "users", title: "Pengguna", icon: "account-group-outline" },
  { name: "settings", title: "Pengaturan", icon: "cog-outline" },
];

export default function TabsLayout() {
  const { session } = useAuth();

  return (
    <Tabs
      screenOptions={{
        headerShown: false,
        tabBarHideOnKeyboard: true,
        tabBarActiveTintColor: colors.primary,
        tabBarInactiveTintColor: colors.textMuted,
        tabBarLabelStyle: {
          fontFamily: typography.bodyMedium,
          fontSize: 11,
        },
        tabBarStyle: {
          height: 68,
          paddingTop: 6,
          paddingBottom: 6,
          borderTopColor: colors.outline,
          backgroundColor: colors.card,
        },
        tabBarItemStyle: {
          minHeight: minimumTouchTarget,
        },
      }}
    >
      {tabs.map((tab) => (
        <Tabs.Screen
          key={tab.name}
          name={tab.name}
          options={{
            title: tab.title,
            ...(tab.name === "users" && session?.user.role !== "superadmin"
              ? { href: null }
              : {}),
            tabBarIcon: ({ color, focused }) => (
              <Icon
                color={String(color)}
                name={
                  focused
                    ? (tab.icon.replace("-outline", "") as IconName)
                    : tab.icon
                }
                size={24}
              />
            ),
          }}
        />
      ))}
    </Tabs>
  );
}
