import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useRef, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { apiRequest } from "@/api/client";
import type { UserListResponse } from "@/api/contracts";
import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field } from "@/components/ui/Field";
import { StateView } from "@/components/ui/StateView";
import type { UserSummary } from "@/domain/types";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";
import { toUserFacingErrorMessage } from "@/utils/errors";
import { initials } from "@/utils/format";

export default function UsersScreen() {
  const router = useRouter();
  const { session } = useAuth();
  const sessionToken = session?.token;
  const sessionUser = session?.user;
  const [users, setUsers] = useState<UserSummary[]>([]);
  const [search, setSearch] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const requestId = useRef(0);

  const load = useCallback(async () => {
    const currentRequestId = ++requestId.current;
    if (!sessionToken || sessionToken.startsWith("dev-only-")) {
      setUsers(sessionUser ? [sessionUser] : []);
      setError(null);
      setLoading(false);
      return;
    }
    setLoading(true);
    try {
      const query = search.trim()
        ? `?search=${encodeURIComponent(search.trim())}`
        : "";
      const response = await apiRequest<UserListResponse>(`/users${query}`, {
        token: sessionToken,
      });
      if (currentRequestId !== requestId.current) return;
      setUsers(response);
      setError(null);
    } catch (reason) {
      if (currentRequestId !== requestId.current) return;
      setError(
        toUserFacingErrorMessage(
          reason,
          "Pengguna belum dapat dimuat. Coba lagi.",
        ),
      );
    } finally {
      if (currentRequestId === requestId.current) setLoading(false);
    }
  }, [search, sessionToken, sessionUser]);

  useFocusEffect(
    useCallback(() => {
      void load();
      return () => {
        requestId.current += 1;
      };
    }, [load]),
  );

  if (session?.user.role !== "superadmin") {
    return (
      <AppScreen>
        <StateView
          icon="shield-lock-outline"
          message="Menu ini hanya tersedia untuk superadmin."
          title="Akses dibatasi"
        />
      </AppScreen>
    );
  }

  return (
    <AppScreen>
      <PageHeader
        subtitle="Akun admin dan superadmin"
        title="Manajemen Pengguna"
      />
      <Field
        label="Cari pengguna"
        onChangeText={setSearch}
        onSubmitEditing={() => void load()}
        placeholder="Nama atau username"
        returnKeyType="search"
        value={search}
      />
      <Button
        icon="account-plus-outline"
        onPress={() => router.push("/users/new")}
      >
        Tambah pengguna
      </Button>
      {error ? (
        <Card style={styles.errorCard}>
          <View style={styles.errorCopy}>
            <Text accessibilityRole="alert" style={styles.errorTitle}>
              Pengguna belum dapat dimuat
            </Text>
            <Text style={styles.errorMessage}>{error}</Text>
          </View>
          <Button
            loading={loading}
            onPress={() => void load()}
            variant="secondary"
          >
            Coba lagi
          </Button>
        </Card>
      ) : null}
      {users.map((user) => (
        <Pressable
          key={user.id}
          onPress={() =>
            router.push({
              pathname: "/users/[id]/edit",
              params: { id: user.id },
            })
          }
        >
          <Card style={styles.user}>
            <View style={styles.avatar}>
              <Text style={styles.avatarText}>{initials(user.fullName)}</Text>
            </View>
            <View style={styles.userCopy}>
              <Text style={styles.userName}>{user.fullName}</Text>
              <Text style={styles.userMeta}>
                @{user.username} • {user.role}
              </Text>
            </View>
            <View
              style={[
                styles.status,
                !user.active && { backgroundColor: colors.container },
              ]}
            >
              <Text
                style={[
                  styles.statusText,
                  !user.active && { color: colors.textMuted },
                ]}
              >
                {user.active ? "AKTIF" : "NONAKTIF"}
              </Text>
            </View>
          </Card>
        </Pressable>
      ))}
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  errorCard: { gap: spacing.md, backgroundColor: colors.errorSoft },
  errorCopy: { gap: spacing.xs },
  errorTitle: { ...textStyles.heading, color: colors.error },
  errorMessage: { ...textStyles.body, color: colors.textMuted },
  user: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  avatar: {
    width: 48,
    height: 48,
    borderRadius: radius.md,
    backgroundColor: colors.primarySoft,
    alignItems: "center",
    justifyContent: "center",
  },
  avatarText: {
    fontFamily: typography.heading,
    color: colors.primary,
    fontSize: 15,
  },
  userCopy: { flex: 1 },
  userName: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    color: colors.text,
  },
  userMeta: { ...textStyles.body, color: colors.textMuted, fontSize: 12 },
  status: {
    backgroundColor: colors.successSoft,
    padding: spacing.sm,
    borderRadius: radius.pill,
  },
  statusText: { ...textStyles.label, color: colors.success, fontSize: 9 },
});
