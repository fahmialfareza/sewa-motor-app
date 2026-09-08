import { StyleSheet } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { PasswordForm } from "@/components/forms/PasswordForm";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import { StateView } from "@/components/ui/StateView";
import { spacing } from "@/theme/tokens";

export default function PasswordSettingsScreen() {
  const { changePassword, session } = useAuth();
  if (session?.dataMode === "sandbox") {
    return (
      <AppScreen>
        <PageHeader back title="Ganti Kata Sandi" />
        <StateView
          icon="shield-lock-outline"
          message="Kata sandi hanya dapat diubah dari Mode Produksi."
          title="Tidak tersedia di Mode Uji"
        />
      </AppScreen>
    );
  }
  return (
    <AppScreen>
      <PageHeader back title="Ganti Kata Sandi" />
      <Card style={styles.form}>
        <PasswordForm onSubmit={changePassword} />
      </Card>
    </AppScreen>
  );
}

const styles = StyleSheet.create({ form: { gap: spacing.md } });
