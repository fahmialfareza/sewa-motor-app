import { StyleSheet, View, type ViewStyle } from "react-native";
import {
  KeyboardAvoidingView,
  KeyboardAwareScrollView,
  KeyboardStickyView,
  type KeyboardAwareScrollViewProps,
} from "react-native-keyboard-controller";
import { SafeAreaView } from "react-native-safe-area-context";

import { colors, minimumTouchTarget, spacing } from "@/theme/tokens";

import { SyncBar } from "./SyncBar";

interface AppScreenProps {
  children?: React.ReactNode;
  authenticated?: boolean;
  scroll?: boolean;
  stickyFooter?: React.ReactNode;
  contentStyle?: ViewStyle;
  scrollProps?: KeyboardAwareScrollViewProps;
}

export function AppScreen({
  children,
  authenticated = true,
  scroll = true,
  stickyFooter,
  contentStyle,
  scrollProps,
}: AppScreenProps) {
  const bottomOffset =
    scrollProps?.bottomOffset ??
    (stickyFooter ? minimumTouchTarget + spacing.xl + spacing.sm : spacing.md);
  const content = scroll ? (
    <KeyboardAwareScrollView
      {...scrollProps}
      bottomOffset={bottomOffset}
      contentContainerStyle={[
        styles.content,
        contentStyle,
        scrollProps?.contentContainerStyle,
      ]}
      keyboardShouldPersistTaps={
        scrollProps?.keyboardShouldPersistTaps ?? "handled"
      }
      showsVerticalScrollIndicator={
        scrollProps?.showsVerticalScrollIndicator ?? false
      }
      style={[styles.flex, scrollProps?.style]}
    >
      {children}
    </KeyboardAwareScrollView>
  ) : (
    <KeyboardAvoidingView
      automaticOffset
      behavior="padding"
      style={styles.flex}
    >
      <View style={[styles.content, styles.flex, contentStyle]}>
        {children}
      </View>
    </KeyboardAvoidingView>
  );

  return (
    <SafeAreaView edges={["top"]} style={styles.safe}>
      {authenticated ? <SyncBar /> : null}
      {content}
      {stickyFooter ? (
        <KeyboardStickyView>
          <View style={styles.footer}>{stickyFooter}</View>
        </KeyboardStickyView>
      ) : null}
    </SafeAreaView>
  );
}

const styles = StyleSheet.create({
  safe: {
    flex: 1,
    backgroundColor: colors.surface,
  },
  flex: { flex: 1 },
  content: {
    padding: spacing.md,
    paddingBottom: spacing.xl,
    gap: spacing.md,
  },
  footer: {
    padding: spacing.md,
    paddingBottom: spacing.md,
    borderTopWidth: 1,
    borderTopColor: colors.outline,
    backgroundColor: colors.card,
  },
});
