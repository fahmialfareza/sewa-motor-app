import { useRouter } from "expo-router";
import { StyleSheet, Text } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { PasswordForm } from "@/components/forms/PasswordForm";
import { AppScreen } from "@/components/layout/AppScreen";
import { Card } from "@/components/ui/Card";
import { colors, spacing, textStyles } from "@/theme/tokens";

export default function ForcedPasswordScreen() {
  const router = useRouter();
  const { changePassword } = useAuth();

  return (
    <AppScreen authenticated={false} contentStyle={styles.screen}>
      <Text style={textStyles.title}>Buat kata sandi baru</Text>
      <Text style={styles.subtitle}>
        Kata sandi sementara harus diganti sebelum terminal dapat digunakan.
        Langkah ini memerlukan internet.
      </Text>
      <Card style={styles.card}>
        <PasswordForm
          onSubmit={async (current, next) => {
            await changePassword(current, next);
            router.replace("/");
          }}
          submitLabel="Aktifkan kata sandi"
        />
      </Card>
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  screen: { flexGrow: 1, justifyContent: "center" },
  subtitle: { ...textStyles.body, color: colors.textMuted },
  card: { gap: spacing.md },
});
