import { useFocusEffect, useLocalSearchParams, useRouter } from "expo-router";
import { useCallback, useRef, useState } from "react";
import { StyleSheet } from "react-native";

import { apiRequest } from "@/api/client";
import { useAuth } from "@/auth/AuthProvider";
import { UserForm, type UserFormValue } from "@/components/forms/UserForm";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import { StateView } from "@/components/ui/StateView";
import type { UserSummary } from "@/domain/types";
import { spacing } from "@/theme/tokens";
import { toUserFacingErrorMessage } from "@/utils/errors";

export default function EditUserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { session } = useAuth();
  const sessionToken = session?.token;
  const sessionUser = session?.user;
  const [user, setUser] = useState<UserSummary | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [loading, setLoading] = useState(true);
  const requestId = useRef(0);

  const load = useCallback(async () => {
    if (!sessionToken || !id) return;
    const currentRequestId = ++requestId.current;
    setLoading(true);
    setLoadError(null);
    try {
      const nextUser =
        sessionToken.startsWith("dev-only-") && sessionUser?.id === id
          ? sessionUser
          : await apiRequest<UserSummary>(`/users/${id}`, {
              token: sessionToken,
            });
      if (!nextUser) throw new Error("Pengguna tidak ditemukan.");
      if (currentRequestId !== requestId.current) return;
      setUser(nextUser);
    } catch (reason) {
      if (currentRequestId !== requestId.current) return;
      setLoadError(
        toUserFacingErrorMessage(
          reason,
          "Data pengguna belum dapat dimuat. Coba lagi.",
        ),
      );
    } finally {
      if (currentRequestId === requestId.current) setLoading(false);
    }
  }, [id, sessionToken, sessionUser]);

  useFocusEffect(
    useCallback(() => {
      void load();
      return () => {
        requestId.current += 1;
      };
    }, [load]),
  );

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
      {loadError && !user ? (
        <StateView
          actionLabel="Coba lagi"
          icon="account-alert-outline"
          message={loadError}
          onAction={() => void load()}
          title="Pengguna belum dapat dimuat"
        />
      ) : user ? (
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
      ) : (
        <StateView
          icon="account-search-outline"
          message="Mohon tunggu sementara data pengguna diambil dari server."
          title={loading ? "Memuat pengguna" : "Pengguna tidak ditemukan"}
        />
      )}
    </AppScreen>
  );
}

const styles = StyleSheet.create({ form: { gap: spacing.md } });
