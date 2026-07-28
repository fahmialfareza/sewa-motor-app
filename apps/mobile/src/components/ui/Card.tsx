import { StyleSheet, View, type ViewProps, type ViewStyle } from "react-native";

import { cardStyle, spacing } from "@/theme/tokens";

interface CardProps extends ViewProps {
  padded?: boolean;
  style?: ViewStyle | ViewStyle[] | undefined;
}

export function Card({ padded = true, style, ...props }: CardProps) {
  return (
    <View style={[styles.card, padded && styles.padded, style]} {...props} />
  );
}

const styles = StyleSheet.create({
  card: cardStyle,
  padded: {
    padding: spacing.md,
  },
});
