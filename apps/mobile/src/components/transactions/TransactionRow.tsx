import { Pressable, StyleSheet, Text, View } from "react-native";

import type { Transaction } from "@/domain/types";
import {
  colors,
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

import { StatusBadge } from "../ui/StatusBadge";
import { PaymentMethodBadge, PaymentStatusBadge } from "../ui/PaymentBadge";

export function TransactionRow({
  transaction,
  onPress,
}: {
  transaction: Transaction;
  onPress: () => void;
}) {
  const accentToken = transaction.items[0]?.accent ?? "primary";
  const accent =
    accentToken === "sunrise"
      ? colors.sunrise
      : accentToken === "standard"
        ? colors.standard
        : colors.primary;

  return (
    <Pressable
      onPress={onPress}
      style={({ pressed }) => [styles.card, pressed && styles.pressed]}
    >
      <View style={[styles.accent, { backgroundColor: accent }]} />
      <View style={styles.content}>
        <View style={styles.top}>
          <View style={styles.flex}>
            <Text style={styles.id}>
              {compactTransactionId(transaction.id)}
            </Text>
            <Text numberOfLines={1} style={styles.name}>
              {transaction.items.map((item) => item.name).join(" + ")}
            </Text>
          </View>
          <Text style={styles.amount}>{formatRupiah(transaction.total)}</Text>
        </View>
        <View style={styles.meta}>
          <Text style={styles.muted}>
            {formatJakartaDateTime(transaction.occurredAt)}
          </Text>
          <Text style={styles.muted}>
            {transaction.items.reduce((sum, item) => sum + item.quantity, 0)}{" "}
            item
          </Text>
        </View>
        <View style={styles.bottom}>
          <Text numberOfLines={1} style={styles.actor}>
            {transaction.updatedActorName}
          </Text>
          <View style={styles.badges}>
            <StatusBadge kind={transaction.syncState} />
            <PaymentMethodBadge method={transaction.paymentMethod} />
            <PaymentStatusBadge status={transaction.paymentStatus} />
          </View>
        </View>
      </View>
    </Pressable>
  );
}

const styles = StyleSheet.create({
  card: {
    backgroundColor: colors.card,
    borderColor: colors.outline,
    borderWidth: 1,
    borderRadius: radius.lg,
    flexDirection: "row",
    overflow: "hidden",
  },
  accent: { width: 5 },
  content: { flex: 1, padding: spacing.md, gap: spacing.sm },
  top: { flexDirection: "row", gap: spacing.sm },
  flex: { flex: 1 },
  id: textStyles.label,
  name: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    color: colors.text,
    marginTop: 2,
  },
  amount: {
    fontFamily: typography.heading,
    fontSize: 16,
    color: colors.primary,
  },
  meta: {
    flexDirection: "row",
    justifyContent: "space-between",
  },
  muted: { ...textStyles.body, color: colors.textMuted, fontSize: 12 },
  bottom: {
    paddingTop: spacing.sm,
    borderTopColor: colors.outline,
    borderTopWidth: StyleSheet.hairlineWidth,
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
  },
  actor: { ...textStyles.body, flex: 1, color: colors.textMuted },
  badges: {
    flexDirection: "row",
    justifyContent: "flex-end",
    gap: spacing.xs,
    flexWrap: "wrap",
  },
  pressed: { opacity: 0.82 },
});
