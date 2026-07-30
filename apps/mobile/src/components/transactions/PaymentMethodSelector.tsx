import { Pressable, StyleSheet, Text, View } from "react-native";

import type { SelectablePaymentMethod } from "@/domain/types";
import {
  colors,
  minimumTouchTarget,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";

import { Icon } from "../ui/Icon";

const methods: {
  value: SelectablePaymentMethod;
  label: string;
  icon: "cash" | "qrcode-scan";
}[] = [
  { value: "cash", label: "Tunai", icon: "cash" },
  { value: "qris", label: "QRIS", icon: "qrcode-scan" },
];

export function PaymentMethodSelector({
  value,
  onChange,
  qrisDisabled = false,
  qrisDisabledReason,
}: {
  value: SelectablePaymentMethod | null;
  onChange: (method: SelectablePaymentMethod) => void;
  qrisDisabled?: boolean;
  qrisDisabledReason?: string;
}) {
  return (
    <View accessibilityLabel="Metode pembayaran" style={styles.container}>
      <Text style={styles.label}>METODE PEMBAYARAN</Text>
      <View style={styles.options}>
        {methods.map((method) => {
          const selected = value === method.value;
          const disabled = method.value === "qris" && qrisDisabled;

          return (
            <Pressable
              accessibilityLabel={method.label}
              accessibilityRole="radio"
              accessibilityState={{ checked: selected, disabled }}
              disabled={disabled}
              key={method.value}
              onPress={() => onChange(method.value)}
              style={({ pressed }) => [
                styles.option,
                selected && styles.optionSelected,
                disabled && styles.optionDisabled,
                pressed && styles.pressed,
              ]}
            >
              <Icon
                color={
                  disabled
                    ? colors.textMuted
                    : selected
                      ? colors.onPrimary
                      : colors.primary
                }
                name={method.icon}
                size={20}
              />
              <Text
                style={[
                  styles.optionText,
                  selected && styles.selectedText,
                  disabled && styles.disabledText,
                ]}
              >
                {method.label}
              </Text>
            </Pressable>
          );
        })}
      </View>
      {qrisDisabled && qrisDisabledReason ? (
        <Text style={styles.disabledReason}>{qrisDisabledReason}</Text>
      ) : null}
    </View>
  );
}

const styles = StyleSheet.create({
  container: { gap: spacing.xs },
  label: { ...textStyles.label, color: colors.textMuted, fontSize: 10 },
  options: { flexDirection: "row", gap: spacing.sm },
  option: {
    minHeight: minimumTouchTarget,
    flex: 1,
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.primary,
    backgroundColor: colors.card,
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "center",
    gap: spacing.sm,
    paddingHorizontal: spacing.sm,
  },
  optionSelected: {
    backgroundColor: colors.primary,
  },
  optionDisabled: {
    borderColor: colors.outline,
    backgroundColor: colors.container,
    opacity: 0.72,
  },
  optionText: {
    fontFamily: typography.bodySemibold,
    fontSize: 14,
    color: colors.primary,
  },
  selectedText: { color: colors.onPrimary },
  disabledText: { color: colors.textMuted },
  disabledReason: {
    ...textStyles.body,
    color: colors.textMuted,
    fontSize: 11,
  },
  pressed: { opacity: 0.82 },
});
