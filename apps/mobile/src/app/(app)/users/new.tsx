import { useRouter } from "expo-router";

import { apiRequest } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { UserForm, type UserFormValue } from "@/components/forms/UserForm";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import { spacing } from "@/theme/tokens";
import { StyleSheet } from "react-native";

export default function NewUserScreen() {
  const router = useRouter();
  const { session } = useAuth();

  const save = async (value: UserFormValue) => {
    if (!session) return;
    await apiRequest("/users", {
      method: "POST",
      token: session.token,
      body: {
        fullName: value.fullName.trim(),
        username: value.username,
        role: value.role,
        temporaryPassword: value.temporaryPassword,
      },
    });
    router.back();
  };

  return (
    <AppScreen>
      <PageHeader back subtitle="Akun online-only" title="Tambah Pengguna" />
      <Card style={styles.form}>
        <UserForm create onSubmit={save} />
      </Card>
    </AppScreen>
  );
}

const styles = StyleSheet.create({ form: { gap: spacing.md } });
