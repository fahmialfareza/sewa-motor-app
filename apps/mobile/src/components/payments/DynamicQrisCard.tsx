import { StyleSheet, Text, View } from "react-native";
import QRCode from "react-native-qrcode-svg";

import { formatRupiah } from "@/utils/format";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";

import { Button } from "../ui/Button";
import { Card } from "../ui/Card";
import { Icon } from "../ui/Icon";

interface DynamicQrisCardProps {
  amount: number;
  merchantName: string | null;
  merchantCity: string | null;
  payload: string | null;
  error: string | null;
  onConfigure?: () => void;
}

export function DynamicQrisCard({
  amount,
  merchantName,
  merchantCity,
  payload,
  error,
  onConfigure,
}: DynamicQrisCardProps) {
  if (!payload || !merchantName || !merchantCity) {
    return (
      <Card style={styles.unavailable}>
        <View style={styles.header}>
          <View style={styles.warningIcon}>
            <Icon color={colors.warning} name="qrcode-remove" size={24} />
          </View>
          <View style={styles.headerCopy}>
            <Text style={styles.eyebrow}>QRIS NOMINAL OTOMATIS</Text>
            <Text style={styles.title}>Kode pembayaran belum tersedia</Text>
          </View>
        </View>
        <Text accessibilityRole="alert" style={styles.guidance}>
          {error ?? "QRIS merchant belum dikonfigurasi pada perangkat ini."}
        </Text>
        {onConfigure ? (
          <Button icon="cog-outline" onPress={onConfigure} variant="secondary">
            Atur QRIS merchant
          </Button>
        ) : null}
      </Card>
    );
  }

  const formattedAmount = formatRupiah(amount);

  return (
    <Card style={styles.ready}>
      <View style={styles.header}>
        <View style={styles.readyIcon}>
          <Icon color={colors.primary} name="qrcode-scan" size={24} />
        </View>
        <View style={styles.headerCopy}>
          <Text style={styles.eyebrow}>QRIS NOMINAL OTOMATIS</Text>
          <Text style={styles.title}>Pindai untuk membayar</Text>
        </View>
      </View>
      <View
        accessibilityLabel={`QRIS pembayaran ${formattedAmount} untuk ${merchantName}`}
        accessibilityRole="image"
        style={styles.qrFrame}
      >
        <QRCode
          backgroundColor="#FFFFFF"
          color="#000000"
          ecl="M"
          quietZone={32}
          size={240}
          value={payload}
        />
      </View>
      <View style={styles.amountBlock}>
        <Text style={styles.amountLabel}>TOTAL PEMBAYARAN</Text>
        <Text style={styles.amount}>{formattedAmount}</Text>
      </View>
      <View style={styles.merchant}>
        <Text style={styles.merchantName}>{merchantName}</Text>
        <Text style={styles.merchantCity}>{merchantCity}</Text>
      </View>
      <Text style={styles.guidance}>
        Cocokkan nama merchant dan nominal di aplikasi pembayaran. Konfirmasi
        berhasil hanya setelah notifikasi diterima merchant.
      </Text>
    </Card>
  );
}

const styles = StyleSheet.create({
  ready: {
    gap: spacing.md,
    alignItems: "stretch",
    borderColor: colors.primary,
    backgroundColor: colors.surfaceBright,
  },
  unavailable: {
    gap: spacing.md,
    backgroundColor: colors.warningSoft,
  },
  header: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
  },
  headerCopy: { flex: 1 },
  readyIcon: {
    width: 44,
    height: 44,
    borderRadius: radius.md,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.primarySoft,
  },
  warningIcon: {
    width: 44,
    height: 44,
    borderRadius: radius.md,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.card,
  },
  eyebrow: { ...textStyles.label, color: colors.primary, fontSize: 10 },
  title: { ...textStyles.heading, marginTop: 2 },
  qrFrame: {
    alignSelf: "center",
    borderRadius: radius.lg,
    backgroundColor: colors.card,
  },
  amountBlock: { alignItems: "center", gap: spacing.xs },
  amountLabel: { ...textStyles.label, fontSize: 10 },
  amount: {
    fontFamily: typography.price,
    fontSize: 28,
    lineHeight: 36,
    color: colors.primary,
  },
  merchant: {
    alignItems: "center",
    gap: 2,
    paddingTop: spacing.sm,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: colors.outline,
  },
  merchantName: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    color: colors.text,
    textAlign: "center",
  },
  merchantCity: {
    ...textStyles.body,
    color: colors.textMuted,
    textAlign: "center",
  },
  guidance: {
    ...textStyles.body,
    color: colors.textMuted,
    textAlign: "center",
    fontSize: 12,
  },
});
