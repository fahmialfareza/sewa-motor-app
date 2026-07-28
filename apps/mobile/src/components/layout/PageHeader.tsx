import { Pressable, StyleSheet, Text, View } from "react-native";
import { useRouter } from "expo-router";

import { colors, spacing, textStyles } from "@/theme/tokens";

import { Icon } from "../ui/Icon";

interface PageHeaderProps {
  title: string;
  subtitle?: string;
  back?: boolean;
  right?: React.ReactNode;
}

export function PageHeader({
  title,
  subtitle,
  back = false,
  right,
}: PageHeaderProps) {
  const router = useRouter();

  return (
    <View style={styles.row}>
      {back ? (
        <Pressable
          accessibilityLabel="Kembali"
          onPress={() => router.back()}
          style={styles.back}
        >
          <Icon color={colors.primary} name="arrow-left" />
        </Pressable>
      ) : null}
      <View style={styles.copy}>
        <Text style={textStyles.title}>{title}</Text>
        {subtitle ? <Text style={styles.subtitle}>{subtitle}</Text> : null}
      </View>
      {right}
    </View>
  );
}

const styles = StyleSheet.create({
  row: {
    minHeight: 56,
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
  },
  back: {
    width: 44,
    height: 44,
    alignItems: "center",
    justifyContent: "center",
    marginLeft: -10,
  },
  copy: {
    flex: 1,
  },
  subtitle: {
    ...textStyles.body,
    color: colors.textMuted,
    marginTop: 2,
  },
});
