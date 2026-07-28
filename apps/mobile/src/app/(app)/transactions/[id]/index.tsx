import { useFocusEffect, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import { StyleSheet, Text, View } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { StateView } from "@/components/ui/StateView";
import { StatusBadge } from "@/components/ui/StatusBadge";
import { getTransaction } from "@/db/repositories";
import { canCorrectTransaction } from "@/domain/permissions";
import type { Transaction } from "@/domain/types";
import { colors, spacing, textStyles, typography } from "@/theme/tokens";
import {
  displayTransactionId,
  formatJakartaDateTime,
  formatRupiah,
} from "@/utils/format";

export default function TransactionDetailScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { session } = useAuth();
  const [transaction, setTransaction] = useState<Transaction | null>(null);
  const [loaded, setLoaded] = useState(false);

  const load = useCallback(async () => {
    if (!id) return;
    setTransaction(await getTransaction(id));
    setLoaded(true);
  }, [id]);

  useFocusEffect(
    useCallback(() => {
      void load();
    }, [load]),
  );

  if (loaded && !transaction) {
    return (
      <AppScreen>
        <PageHeader back title="Detail transaksi" />
        <StateView
          icon="receipt-text-remove-outline"
          message="Data mungkin telah dihapus di server dan tombstone sudah diterima."
          title="Transaksi tidak ditemukan"
        />
      </AppScreen>
    );
  }
  if (!transaction) return <AppScreen />;

  const mayCorrect = canCorrectTransaction(session, transaction);

  return (
    <AppScreen>
      <PageHeader
        back
        subtitle={formatJakartaDateTime(transaction.occurredAt)}
        title={displayTransactionId(transaction.id)}
      />
      <View style={styles.badges}>
        <StatusBadge kind={transaction.syncState} />
        <StatusBadge kind={transaction.printState} />
      </View>
      {transaction.syncState === "conflict" ? (
        <Card style={styles.conflict}>
          <Text style={styles.conflictTitle}>Koreksi perlu ditinjau</Text>
          <Text style={styles.muted}>
            Versi server berubah sebelum koreksi perangkat ini diterima.
          </Text>
          <Button
            onPress={() =>
              router.push({
                pathname: "/transactions/[id]/conflict",
                params: { id: transaction.id },
              })
            }
            variant="secondary"
          >
            Bandingkan versi
          </Button>
        </Card>
      ) : null}

      <Card style={styles.receipt}>
        <View style={styles.metaRow}>
          <View>
            <Text style={textStyles.label}>KASIR ASAL</Text>
            <Text style={styles.metaValue}>{transaction.originActorName}</Text>
          </View>
          <View style={styles.alignRight}>
            <Text style={textStyles.label}>REVISI</Text>
            <Text style={styles.metaValue}>#{transaction.revision}</Text>
          </View>
        </View>
        <View style={styles.rule} />
        {transaction.items.map((item) => (
          <View key={item.id} style={styles.item}>
            <View style={styles.itemCopy}>
              <Text style={styles.itemName}>{item.name}</Text>
              <Text style={styles.muted}>
                {item.quantity} × {formatRupiah(item.unitPrice)}
              </Text>
              <Text style={styles.snapshot}>
                SNAPSHOT PAKET REV. {item.packageRevision}
              </Text>
            </View>
            <Text style={styles.itemTotal}>{formatRupiah(item.lineTotal)}</Text>
          </View>
        ))}
        <View style={styles.rule} />
        <View style={styles.total}>
          <Text style={styles.totalLabel}>Total</Text>
          <Text style={textStyles.price}>
            {formatRupiah(transaction.total)}
          </Text>
        </View>
      </Card>

      <Card style={styles.audit}>
        <Text style={textStyles.label}>JEJAK PERUBAHAN</Text>
        <Text style={styles.muted}>
          Dibuat oleh {transaction.originActorName}
        </Text>
        <Text style={styles.muted}>
          Terakhir diperbarui oleh {transaction.updatedActorName}
        </Text>
        <Text style={styles.terminal} numberOfLines={1}>
          TERMINAL {transaction.terminalId}
        </Text>
      </Card>

      {mayCorrect ? (
        <Button
          icon="pencil-outline"
          onPress={() =>
            router.push({
              pathname: "/transactions/[id]/correct",
              params: { id: transaction.id },
            })
          }
          variant="secondary"
        >
          Koreksi transaksi
        </Button>
      ) : null}
      <Button
        icon="printer-outline"
        onPress={() =>
          router.push({
            pathname: "/transactions/[id]/print",
            params: { id: transaction.id },
          })
        }
      >
        {transaction.printState === "success" ||
        transaction.printState === "unknown" ||
        transaction.printState === "needs-reprint"
          ? "Cetak salinan"
          : "Cetak struk"}
      </Button>
      {session?.user.role === "superadmin" ? (
        <Text style={styles.onlineOnly}>
          Penghapusan transaksi hanya tersedia saat online dan dilakukan dari
          aksi server setelah alasan dikonfirmasi.
        </Text>
      ) : null}
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  badges: { flexDirection: "row", gap: spacing.sm, flexWrap: "wrap" },
  conflict: { gap: spacing.sm, backgroundColor: colors.warningSoft },
  conflictTitle: { ...textStyles.heading, color: colors.warning },
  muted: { ...textStyles.body, color: colors.textMuted },
  receipt: { gap: spacing.md },
  metaRow: { flexDirection: "row", justifyContent: "space-between" },
  metaValue: {
    fontFamily: typography.bodySemibold,
    color: colors.text,
    marginTop: spacing.xs,
  },
  alignRight: { alignItems: "flex-end" },
  rule: { height: 1, backgroundColor: colors.outline },
  item: { flexDirection: "row", gap: spacing.sm },
  itemCopy: { flex: 1 },
  itemName: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    color: colors.text,
  },
  itemTotal: {
    fontFamily: typography.heading,
    fontSize: 16,
    color: colors.primary,
  },
  snapshot: { ...textStyles.label, marginTop: spacing.xs, fontSize: 9 },
  total: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  totalLabel: { ...textStyles.heading },
  audit: { gap: spacing.xs, backgroundColor: colors.surfaceBright },
  terminal: { ...textStyles.label, fontSize: 9 },
  onlineOnly: {
    ...textStyles.body,
    color: colors.textMuted,
    textAlign: "center",
    fontSize: 12,
  },
});
