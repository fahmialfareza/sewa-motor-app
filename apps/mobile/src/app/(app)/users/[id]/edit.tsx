import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { StyleSheet } from "react-native";

import { apiRequest } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { UserForm, type UserFormValue } from "@/components/forms/UserForm";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import type { UserSummary } from "@/domain/types";
import { spacing } from "@/theme/tokens";

export default function EditUserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { session } = useAuth();
  const [user, setUser] = useState<UserSummary | null>(null);

  useEffect(() => {
    if (!session || !id) return;
    const userRequest =
      session.token.startsWith("dev-only-") && session.user.id === id
        ? Promise.resolve(session.user)
        : apiRequest<UserSummary>(`/users/${id}`, {
            token: session.token,
          });
    void userRequest.then(setUser);
  }, [id, session]);

  const save = async (value: UserFormValue) => {
    if (!session || !id) return;
    if (!session.token.startsWith("dev-only-")) {
      await apiRequest(`/users/${id}`, {
        method: "PATCH",
        token: session.token,
        body: {
          fullName: value.fullName.trim(),
          username: value.username,
          role: value.role,
          active: value.active,
        },
      });
    }
    router.back();
  };

  return (
    <AppScreen>
      <PageHeader
        back
        subtitle="Perlindungan akun diterapkan server"
        title="Edit Pengguna"
      />
      {user ? (
        <Card style={styles.form}>
          <UserForm
            create={false}
            initial={{
              fullName: user.fullName,
              username: user.username,
              role: user.role,
              active: user.active,
            }}
            onSubmit={save}
          />
        </Card>
      ) : null}
    </AppScreen>
  );
}

const styles = StyleSheet.create({ form: { gap: spacing.md } });
