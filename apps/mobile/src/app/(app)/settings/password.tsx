import { StyleSheet } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { PasswordForm } from "@/components/forms/PasswordForm";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import { spacing } from "@/theme/tokens";

export default function PasswordSettingsScreen() {
  const { changePassword } = useAuth();
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
