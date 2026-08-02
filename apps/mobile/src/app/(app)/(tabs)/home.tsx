import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useEffect, useRef, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { DateMonthFilter } from "@/components/reporting/DateMonthFilter";
import { TransactionRow } from "@/components/transactions/TransactionRow";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { getDashboardStats, listTransactions } from "@/db/repositories";
import type { DashboardStats, Transaction } from "@/domain/types";
import { useSyncRuntime } from "@/sync/SyncProvider";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";
import { formatRupiah, initials } from "@/utils/format";
import {
  currentJakartaDate,
  currentJakartaMonth,
  reportingRange,
  type ReportingMode,
} from "@/utils/time";

const emptyStats: DashboardStats = {
  gross: 0,
  transactionCount: 0,
  quantities: [],
  buckets: Array(24).fill(0),
};

export default function HomeScreen() {
  const router = useRouter();
  const { session } = useAuth();
  const { lastSyncedAt, pendingCount } = useSyncRuntime();
  const [mode, setMode] = useState<ReportingMode>("date");
  const [selectedDate, setSelectedDate] = useState(currentJakartaDate);
  const [selectedMonth, setSelectedMonth] = useState(currentJakartaMonth);
  const [stats, setStats] = useState(emptyStats);
  const [recent, setRecent] = useState<Transaction[]>([]);
  const [loaded, setLoaded] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const requestId = useRef(0);
  const observedSyncAt = useRef(lastSyncedAt);

  const load = useCallback(async () => {
    const currentRequestId = ++requestId.current;
    const range =
      mode === "date"
        ? reportingRange("date", selectedDate)
        : reportingRange("month", selectedMonth);
    setLoaded(false);
    setLoadError(null);
    try {
      const [nextStats, nextRecent] = await Promise.all([
        getDashboardStats(range),
        listTransactions({ limit: 5 }),
      ]);
      if (currentRequestId !== requestId.current) return;
      setStats(nextStats);
      setRecent(nextRecent);
      setLoaded(true);
      setLoadError(null);
    } catch (reason) {
      if (currentRequestId !== requestId.current) return;
      setLoadError(
        reason instanceof Error
          ? reason.message
          : "Ringkasan dasbor tidak dapat dimuat.",
      );
    }
  }, [mode, selectedDate, selectedMonth]);

  useFocusEffect(
    useCallback(() => {
      void load();
      return () => {
        requestId.current += 1;
      };
    }, [load]),
  );

  useEffect(() => {
    if (lastSyncedAt === observedSyncAt.current) return;
    observedSyncAt.current = lastSyncedAt;
    void load();
  }, [lastSyncedAt, load]);

  const maximum = Math.max(...stats.buckets, 1);

  return (
    <AppScreen>
      <PageHeader
        subtitle="Ringkasan transaksi dari perangkat ini"
        title="Dasbor"
        right={
          <View style={styles.avatar}>
            <Text style={styles.avatarText}>
              {initials(session?.user.fullName ?? "POS")}
            </Text>
          </View>
        }
      />
      <DateMonthFilter
        date={selectedDate}
        mode={mode}
        month={selectedMonth}
        onDateChange={setSelectedDate}
        onModeChange={setMode}
        onMonthChange={setSelectedMonth}
      />
      <Button
        icon="plus-circle-outline"
        onPress={() => router.push("/(app)/(tabs)/sell")}
      >
        Mulai transaksi baru
      </Button>

      {loadError ? (
        <Card style={styles.loadError}>
          <View style={styles.loadErrorCopy}>
            <Text accessibilityRole="alert" style={styles.loadErrorTitle}>
              Ringkasan belum diperbarui
            </Text>
            <Text style={styles.loadErrorMessage}>{loadError}</Text>
          </View>
          <Button onPress={() => void load()} variant="secondary">
            Coba lagi
          </Button>
        </Card>
      ) : null}

      <Card style={styles.revenue}>
        <View style={styles.metricTop}>
          <View>
            <Text style={textStyles.label}>PENDAPATAN KOTOR</Text>
            <Text style={textStyles.price}>
              {loaded ? formatRupiah(stats.gross) : "—"}
            </Text>
          </View>
          <View style={styles.countPill}>
            <Text style={styles.countText}>
              {loaded ? `${stats.transactionCount} transaksi lunas` : "Memuat…"}
            </Text>
          </View>
        </View>
        <Text style={styles.revenueNote}>
          Menghitung pembayaran berhasil pada revisi transaksi saat ini.
        </Text>
        <View style={styles.chart}>
          {stats.buckets.map((amount, index) => (
            <View
              key={`${index}-${amount}`}
              style={[
                styles.bar,
                {
                  height: Math.max(8, (amount / maximum) * 64),
                  backgroundColor:
                    index === stats.buckets.length - 1
                      ? colors.primary
                      : colors.primarySoft,
                },
              ]}
            />
          ))}
        </View>
      </Card>

      <View style={styles.metricGrid}>
        {!loaded ? (
          <Card style={styles.smallMetric}>
            <Text style={styles.metricLabel}>Paket terjual</Text>
            <Text style={styles.metricValue}>Memuat…</Text>
          </Card>
        ) : stats.quantities.length > 0 ? (
          stats.quantities.map((quantity) => (
            <Card key={quantity.name} style={styles.smallMetric}>
              <View
                style={[
                  styles.packageDot,
                  {
                    backgroundColor:
                      quantity.accent === "sunrise"
                        ? colors.sunrise
                        : quantity.accent === "standard"
                          ? colors.standard
                          : colors.primary,
                  },
                ]}
              />
              <Text style={styles.metricLabel}>{quantity.name}</Text>
              <Text style={styles.metricValue}>{quantity.quantity} unit</Text>
            </Card>
          ))
        ) : (
          <Card style={styles.smallMetric}>
            <Text style={styles.metricLabel}>Paket terjual</Text>
            <Text style={styles.metricValue}>Belum ada</Text>
          </Card>
        )}
        <Card style={styles.smallMetric}>
          <Text style={styles.metricLabel}>Belum sinkron</Text>
          <Text
            style={[
              styles.metricValue,
              pendingCount > 0 && { color: colors.warning },
            ]}
          >
            {pendingCount} perubahan
          </Text>
        </Card>
      </View>

      <View style={styles.sectionHeader}>
        <Text style={textStyles.heading}>Transaksi terkini</Text>
        <Pressable onPress={() => router.push("/(app)/(tabs)/history")}>
          <Text style={styles.link}>Lihat semua</Text>
        </Pressable>
      </View>
      {recent.length === 0 ? (
        <Card>
          <Text style={styles.empty}>
            Transaksi yang disimpan akan langsung muncul di sini, termasuk saat
            offline.
          </Text>
        </Card>
      ) : (
        recent.map((transaction) => (
          <TransactionRow
            key={transaction.id}
            onPress={() =>
              router.push({
                pathname: "/transactions/[id]",
                params: { id: transaction.id },
              })
            }
            transaction={transaction}
          />
        ))
      )}
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  avatar: {
    width: 44,
    height: 44,
    borderRadius: radius.md,
    backgroundColor: colors.primary,
    justifyContent: "center",
    alignItems: "center",
  },
  avatarText: {
    color: colors.onPrimary,
    fontFamily: typography.heading,
    fontSize: 16,
  },
  revenue: { gap: spacing.lg },
  revenueNote: { ...textStyles.body, color: colors.textMuted, fontSize: 12 },
  loadError: {
    gap: spacing.md,
    backgroundColor: colors.errorSoft,
  },
  loadErrorCopy: { gap: spacing.xs },
  loadErrorTitle: { ...textStyles.heading, color: colors.error },
  loadErrorMessage: { ...textStyles.body, color: colors.textMuted },
  metricTop: {
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "flex-start",
  },
  countPill: {
    backgroundColor: colors.successSoft,
    padding: spacing.sm,
    borderRadius: radius.md,
  },
  countText: { ...textStyles.body, color: colors.success, fontSize: 12 },
  chart: {
    height: 68,
    flexDirection: "row",
    alignItems: "flex-end",
    gap: spacing.sm,
  },
  bar: { flex: 1, borderRadius: radius.sm },
  metricGrid: { flexDirection: "row", flexWrap: "wrap", gap: spacing.sm },
  smallMetric: { flexGrow: 1, flexBasis: "45%", gap: spacing.xs },
  packageDot: { width: 28, height: 4, borderRadius: radius.pill },
  metricLabel: { ...textStyles.label, textTransform: "uppercase" },
  metricValue: {
    fontFamily: typography.heading,
    fontSize: 20,
    color: colors.text,
  },
  sectionHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  link: { ...textStyles.body, color: colors.primary },
  empty: { ...textStyles.body, color: colors.textMuted },
});
