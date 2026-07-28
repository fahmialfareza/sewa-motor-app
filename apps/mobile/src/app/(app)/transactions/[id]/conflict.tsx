import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { StyleSheet, Text, View } from "react-native";

import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { getConflictForTransaction, resolveConflict } from "@/db/repositories";
import type { SyncConflict, Transaction } from "@/domain/types";
import { useSyncRuntime } from "@/sync/SyncProvider";
import { colors, spacing, textStyles, typography } from "@/theme/tokens";
import { formatRupiah } from "@/utils/format";

export default function ConflictReviewScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const sync = useSyncRuntime();
  const [conflict, setConflict] = useState<SyncConflict | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    if (id) void getConflictForTransaction(id).then(setConflict);
  }, [id]);

  const decide = async (resolution: "server" | "retry-local") => {
    if (!conflict) return;
    setBusy(true);
    try {
      await resolveConflict(conflict, resolution);
      await sync.refresh();
      if (resolution === "retry-local") void sync.syncNow();
      router.replace({
        pathname: "/transactions/[id]",
        params: { id: conflict.transactionId },
      });
    } finally {
      setBusy(false);
    }
  };

  if (!conflict) {
    return (
      <AppScreen>
        <PageHeader back title="Tinjau konflik" />
        <Text style={styles.muted}>
          Konflik sudah diselesaikan atau belum tersedia.
        </Text>
      </AppScreen>
    );
  }

  return (
    <AppScreen>
      <PageHeader
        back
        subtitle="Pilih hasil setelah membandingkan kedua versi"
        title="Konflik Revisi"
      />
      <View style={styles.columns}>
        <SnapshotCard
          label="VERSI PERANGKAT"
          tone="local"
          transaction={conflict.localSnapshot}
        />
        <SnapshotCard
          label="VERSI SERVER"
          tone="server"
          transaction={conflict.serverSnapshot}
        />
      </View>
      <Card style={styles.warning}>
        <Text style={styles.warningTitle}>Keputusan manual diperlukan</Text>
        <Text style={styles.muted}>
          “Kirim ulang lokal” membuat operasi baru dengan base revision server
          dan tanda tangan baru. Pastikan jumlah lokal memang yang benar.
        </Text>
      </Card>
      <Button loading={busy} onPress={() => void decide("retry-local")}>
        Kirim ulang versi lokal
      </Button>
      <Button
        disabled={busy}
        onPress={() => void decide("server")}
        variant="secondary"
      >
        Gunakan versi server
      </Button>
    </AppScreen>
  );
}

function SnapshotCard({
  label,
  transaction,
  tone,
}: {
  label: string;
  transaction: Transaction;
  tone: "local" | "server";
}) {
  return (
    <Card
      style={[
        styles.snapshot,
        {
          borderColor: tone === "local" ? colors.warning : colors.secondary,
        },
      ]}
    >
      <Text
        style={[
          textStyles.label,
          { color: tone === "local" ? colors.warning : colors.secondary },
        ]}
      >
        {label}
      </Text>
      <Text style={styles.revision}>Revisi #{transaction.revision}</Text>
      {transaction.items.map((item) => (
        <View key={item.packageId} style={styles.snapshotLine}>
          <Text style={styles.itemName}>{item.name}</Text>
          <Text style={styles.quantity}>× {item.quantity}</Text>
        </View>
      ))}
      <Text style={styles.amount}>{formatRupiah(transaction.total)}</Text>
    </Card>
  );
}

const styles = StyleSheet.create({
  columns: { gap: spacing.md },
  snapshot: { gap: spacing.sm, borderWidth: 2 },
  revision: {
    fontFamily: typography.heading,
    fontSize: 18,
    color: colors.text,
  },
  snapshotLine: { flexDirection: "row", justifyContent: "space-between" },
  itemName: { ...textStyles.body, flex: 1 },
  quantity: { ...textStyles.body, fontFamily: typography.bodySemibold },
  amount: {
    fontFamily: typography.heading,
    fontSize: 20,
    color: colors.primary,
    textAlign: "right",
  },
  warning: { gap: spacing.xs, backgroundColor: colors.warningSoft },
  warningTitle: { ...textStyles.heading, color: colors.warning },
  muted: { ...textStyles.body, color: colors.textMuted },
});
