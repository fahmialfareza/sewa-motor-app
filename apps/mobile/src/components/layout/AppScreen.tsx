import {
  Pressable,
  StyleSheet,
  Text,
  View,
  type ViewStyle,
} from "react-native";
import {
  KeyboardAvoidingView,
  KeyboardAwareScrollView,
  KeyboardStickyView,
  type KeyboardAwareScrollViewProps,
} from "react-native-keyboard-controller";
import { SafeAreaView } from "react-native-safe-area-context";

import { useAuthStore } from "@/auth/auth-store";
import { useModeStore } from "@/mode/mode-store";
import {
  colors,
  minimumTouchTarget,
  spacing,
  typography,
} from "@/theme/tokens";

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
  const sandbox = useModeStore((state) => state.dataMode === "sandbox");
  const notice = useAuthStore((state) => state.notice);
  const dismissNotice = useAuthStore((state) => state.dismissNotice);
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
      {authenticated && sandbox ? (
        <View accessibilityRole="alert" style={styles.sandboxBanner}>
          <Text style={styles.sandboxBannerText}>
            MODE UJI — DATA TIDAK MASUK LAPORAN PRODUKSI
          </Text>
        </View>
      ) : null}
      {authenticated && notice ? (
        <View accessibilityRole="alert" style={styles.recoveryNotice}>
          <Text style={styles.recoveryNoticeText}>{notice}</Text>
          <Pressable
            accessibilityLabel="Tutup pemberitahuan"
            accessibilityRole="button"
            hitSlop={spacing.sm}
            onPress={() => void dismissNotice().catch(() => undefined)}
          >
            <Text style={styles.recoveryNoticeDismiss}>Tutup</Text>
          </Pressable>
        </View>
      ) : null}
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
  sandboxBanner: {
    minHeight: 34,
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.xs,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.warningSoft,
    borderBottomWidth: 1,
    borderBottomColor: colors.warning,
  },
  sandboxBannerText: {
    color: colors.warning,
    fontFamily: typography.bodySemibold,
    fontSize: 11,
    textAlign: "center",
  },
  recoveryNotice: {
    paddingHorizontal: spacing.md,
    paddingVertical: spacing.sm,
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    backgroundColor: colors.primarySoft,
    borderBottomWidth: 1,
    borderBottomColor: colors.primary,
  },
  recoveryNoticeText: {
    flex: 1,
    color: colors.text,
    fontFamily: typography.body,
    fontSize: 12,
  },
  recoveryNoticeDismiss: {
    color: colors.primary,
    fontFamily: typography.bodySemibold,
    fontSize: 12,
  },
});
