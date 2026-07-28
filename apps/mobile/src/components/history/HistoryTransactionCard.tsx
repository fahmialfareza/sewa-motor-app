import { Pressable, StyleSheet, Text, View } from "react-native";

import type { PrintState, SyncState, Transaction } from "@/domain/types";
import {
  colors,
  minimumTouchTarget,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";
import {
  compactTransactionId,
  formatJakartaDateTime,
  formatRupiah,
} from "@/utils/format";

import { Icon } from "../ui/Icon";
import { StatusBadge } from "../ui/StatusBadge";

const syncStateLabel: Record<SyncState, string> = {
  pending: "menunggu sinkronisasi",
  synced: "tersinkron",
  conflict: "konflik",
  error: "gagal disinkronkan",
};

const printStateLabel: Record<PrintState, string> = {
  pending: "belum dicetak",
  success: "tercetak",
  failed: "gagal dicetak",
  unknown: "hasil cetak tidak diketahui",
  "needs-reprint": "perlu dicetak ulang",
};

export function HistoryTransactionCard({
  transaction,
  onPress,
}: {
  transaction: Transaction;
  onPress: () => void;
}) {
  const transactionId = compactTransactionId(transaction.id);
  const itemQuantity = transaction.items.reduce(
    (sum, item) => sum + item.quantity,
    0,
  );
  const packageNames = transaction.items.map((item) => item.name).join(" + ");
  const accentToken = transaction.items[0]?.accent ?? "primary";
  const accent =
    accentToken === "sunrise"
      ? colors.sunrise
      : accentToken === "standard"
        ? colors.standard
        : colors.primary;
  const accessibilityLabel = [
    `Buka transaksi ${transactionId}`,
    packageNames,
    formatRupiah(transaction.total),
    formatJakartaDateTime(transaction.occurredAt),
    `${itemQuantity} item`,
    `dibuat oleh ${transaction.updatedActorName}`,
    syncStateLabel[transaction.syncState],
    printStateLabel[transaction.printState],
  ].join(", ");

  return (
    <Pressable
      accessibilityHint="Membuka detail transaksi"
      accessibilityLabel={accessibilityLabel}
      accessibilityRole="button"
      onPress={onPress}
      style={({ pressed }) => [styles.card, pressed && styles.pressed]}
    >
      <View style={[styles.accent, { backgroundColor: accent }]} />
      <View style={styles.content}>
        <View style={styles.header}>
          <Text numberOfLines={1} style={styles.id}>
            {transactionId}
          </Text>
          <StatusBadge kind={transaction.syncState} />
        </View>

        <View style={styles.summary}>
          <View style={styles.summaryCopy}>
            <Text numberOfLines={1} style={styles.packageNames}>
              {packageNames}
            </Text>
            <Text style={styles.date}>
              {formatJakartaDateTime(transaction.occurredAt)} · {itemQuantity}{" "}
              item
            </Text>
          </View>
          <Text numberOfLines={1} style={styles.amount}>
            {formatRupiah(transaction.total)}
          </Text>
        </View>

        <View style={styles.footer}>
          <View style={styles.actor}>
            <Icon color={colors.textMuted} name="account-outline" size={18} />
            <Text numberOfLines={1} style={styles.actorName}>
              {transaction.updatedActorName}
            </Text>
          </View>
          {transaction.printState !== "pending" ? (
            <StatusBadge kind={transaction.printState} />
          ) : null}
          <View style={styles.chevron}>
            <Icon color={colors.primary} name="chevron-right" size={22} />
          </View>
        </View>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  card: {
    minHeight: minimumTouchTarget,
    backgroundColor: colors.card,
    borderColor: colors.outline,
    borderWidth: 1,
    borderRadius: radius.lg,
    flexDirection: "row",
    overflow: "hidden",
  },
  accent: {
    width: 4,
  },
  content: {
    flex: 1,
    padding: spacing.md,
    gap: spacing.sm,
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: spacing.sm,
  },
  id: {
    ...textStyles.label,
    color: colors.textMuted,
    flex: 1,
  },
  summary: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: spacing.md,
  },
  summaryCopy: {
    flex: 1,
    gap: 2,
  },
  packageNames: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    lineHeight: 22,
    color: colors.text,
  },
  date: {
    ...textStyles.body,
    color: colors.textMuted,
    fontSize: 12,
  },
  amount: {
    fontFamily: typography.heading,
    fontSize: 17,
    lineHeight: 22,
    color: colors.primary,
    textAlign: "right",
  },
  footer: {
    minHeight: 28,
    paddingTop: spacing.sm,
    borderTopColor: colors.outline,
    borderTopWidth: StyleSheet.hairlineWidth,
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
  },
  actor: {
    flex: 1,
    minWidth: 0,
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.xs,
  },
  actorName: {
    ...textStyles.body,
    color: colors.textMuted,
    flexShrink: 1,
  },
  chevron: {
    width: 24,
    alignItems: "flex-end",
  },
  pressed: {
    backgroundColor: colors.surfaceBright,
    opacity: 0.86,
  },
});
