import { forwardRef, useState } from "react";
import {
  Pressable,
  StyleSheet,
  Text,
  TextInput,
  View,
  type TextInputProps,
} from "react-native";

import {
  colors,
  minimumTouchTarget,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";

import { Icon } from "./Icon";

interface FieldProps extends TextInputProps {
  label: string;
  error?: string | undefined;
  hint?: string;
}

export const Field = forwardRef<TextInput, FieldProps>(function Field(
  { label, error, hint, secureTextEntry, style, ...props },
  ref,
) {
  const [passwordVisible, setPasswordVisible] = useState(false);
  const isPasswordField = secureTextEntry === true;

  return (
    <View style={styles.wrapper}>
      <Text style={styles.label}>{label.toUpperCase()}</Text>
      <View>
        <TextInput
          ref={ref}
          placeholderTextColor={colors.textMuted}
          selectionColor={colors.primary}
          style={[
            styles.input,
            error && styles.inputError,
            style,
            isPasswordField && styles.passwordInput,
          ]}
          {...props}
          secureTextEntry={isPasswordField ? !passwordVisible : secureTextEntry}
        />
        {isPasswordField ? (
          <Pressable
            accessibilityLabel={
              passwordVisible
                ? "Sembunyikan kata sandi"
                : "Tampilkan kata sandi"
            }
            accessibilityRole="button"
            onPress={() => setPasswordVisible((visible) => !visible)}
            style={({ pressed }) => [
              styles.passwordToggle,
              pressed && styles.passwordTogglePressed,
            ]}
          >
            <Icon
              color={colors.textMuted}
              name={passwordVisible ? "eye-off-outline" : "eye-outline"}
              size={22}
            />
          </Pressable>
        ) : null}
      </View>
      {error ? <Text style={styles.error}>{error}</Text> : null}
      {!error && hint ? <Text style={styles.hint}>{hint}</Text> : null}
    </View>
  );
});

const styles = StyleSheet.create({
  wrapper: {
    gap: spacing.xs,
  },
  label: textStyles.label,
  input: {
    minHeight: minimumTouchTarget,
    borderWidth: 1,
    borderColor: colors.outline,
    borderRadius: radius.md,
    paddingHorizontal: spacing.md,
    backgroundColor: colors.card,
    color: colors.text,
    fontFamily: typography.body,
    fontSize: 16,
  },
  passwordInput: {
    paddingRight: minimumTouchTarget + spacing.sm,
  },
  passwordToggle: {
    position: "absolute",
    top: 0,
    right: 0,
    bottom: 0,
    minWidth: minimumTouchTarget,
    alignItems: "center",
    justifyContent: "center",
  },
  passwordTogglePressed: {
    opacity: 0.6,
  },
  inputError: {
    borderColor: colors.error,
  },
  error: {
    ...textStyles.body,
    color: colors.error,
    fontSize: 12,
  },
  hint: {
    ...textStyles.body,
    color: colors.textMuted,
    fontSize: 12,
  },
});
