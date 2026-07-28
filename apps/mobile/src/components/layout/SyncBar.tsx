import { Pressable, StyleSheet, Text, View } from "react-native";
import { useRouter } from "expo-router";

import { useSyncRuntime } from "@/sync/SyncProvider";
import { colors, spacing, typography } from "@/theme/tokens";

import { Icon } from "../ui/Icon";

export function SyncBar() {
  const router = useRouter();
  const { online, pendingCount, syncing, lastSyncedAt } = useSyncRuntime();

  const label = online
    ? pendingCount > 0
      ? `${pendingCount} perubahan menunggu`
      : lastSyncedAt
        ? `Sinkron ${relativeTime(lastSyncedAt)}`
        : "Siap sinkron"
    : "Offline — data tetap tersimpan";

  return (
    <Pressable
      accessibilityRole="button"
      onPress={() => router.push("/sync")}
      style={[styles.bar, !online && styles.offline]}
    >
      <View style={styles.left}>
        <View
          style={[styles.dot, online ? styles.dotOnline : styles.dotOffline]}
        />
        <Text numberOfLines={1} style={styles.text}>
          {online ? "SISTEM ONLINE" : "MODE OFFLINE"}
        </Text>
      </View>
      <View style={styles.right}>
        <Text numberOfLines={1} style={styles.detail}>
          {label}
        </Text>
        <Icon
          color={colors.onPrimary}
          name={syncing ? "sync" : "chevron-right"}
          size={16}
        />
      </View>
    </Pressable>
  );
}

function relativeTime(iso: string): string {
  const minutes = Math.max(
    0,
    Math.floor((Date.now() - new Date(iso).getTime()) / 60_000),
  );
  if (minutes < 1) return "baru saja";
  if (minutes < 60) return `${minutes} mnt lalu`;
  return `${Math.floor(minutes / 60)} jam lalu`;
}

const styles = StyleSheet.create({
  bar: {
    minHeight: 36,
    paddingHorizontal: spacing.md,
    backgroundColor: colors.primary,
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
    gap: spacing.sm,
  },
  offline: {
    backgroundColor: colors.warning,
  },
  left: {
    flexDirection: "row",
    alignItems: "center",
    gap: 6,
  },
  right: {
    flex: 1,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "flex-end",
    gap: 2,
  },
  dot: { width: 7, height: 7, borderRadius: 4 },
  dotOnline: { backgroundColor: "#65E6AD" },
  dotOffline: { backgroundColor: "#FFE1A8" },
  text: {
    color: colors.onPrimary,
    fontFamily: typography.mono,
    fontSize: 10,
  },
  detail: {
    color: colors.onPrimary,
    fontFamily: typography.body,
    fontSize: 11,
    maxWidth: "80%",
  },
});
