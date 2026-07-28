import { useRouter } from "expo-router";
import { useState } from "react";
import { StyleSheet, Text, View } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import { MenuRow } from "@/components/ui/MenuRow";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";
import { initials } from "@/utils/format";

export default function SettingsScreen() {
  const router = useRouter();
  const { session, logout } = useAuth();
  const [error, setError] = useState<string | null>(null);

  return (
    <AppScreen>
      <PageHeader title="Pengaturan" />
      <Card style={styles.profile}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>
            {initials(session?.user.fullName ?? "POS")}
          </Text>
        </View>
        <View style={styles.profileCopy}>
          <Text style={styles.name}>{session?.user.fullName}</Text>
          <Text style={styles.username}>@{session?.user.username}</Text>
          <View style={styles.role}>
            <Text style={styles.roleText}>
              {session?.user.role === "superadmin" ? "SUPERADMIN" : "ADMIN"}
            </Text>
          </View>
        </View>
      </Card>

      <Text style={styles.section}>AKUN & KEAMANAN</Text>
      <Card padded={false}>
        <MenuRow
          icon="account-outline"
          onPress={() => router.push("/settings/profile")}
          title="Profil"
        />
        <MenuRow
          icon="lock-outline"
          onPress={() => router.push("/settings/password")}
          title="Ganti kata sandi"
        />
      </Card>
      <Text style={styles.section}>TERMINAL</Text>
      <Card padded={false}>
        <MenuRow
          detail="Bluetooth, printer MPOS, dan lebar kertas"
          icon="printer-outline"
          onPress={() => router.push("/settings/printer")}
          title="Pengaturan printer"
        />
        <MenuRow
          detail="Outbox, konflik, dan sinkron manual"
          icon="sync"
          onPress={() => router.push("/sync")}
          title="Sinkronisasi data"
        />
        {session?.user.role === "superadmin" ? (
          <MenuRow
            detail="Harga baru hanya berlaku untuk transaksi baru"
            icon="tag-outline"
            onPress={() => router.push("/packages")}
            title="Paket & harga"
          />
        ) : null}
      </Card>
      <Card padded={false}>
        <MenuRow
          destructive
          icon="logout"
          onPress={() => {
            setError(null);
            void logout().catch((reason: unknown) =>
              setError(
                reason instanceof Error ? reason.message : "Logout gagal.",
              ),
            );
          }}
          title="Keluar"
        />
      </Card>
      {error ? <Text style={styles.error}>{error}</Text> : null}
      <Text style={styles.version}>SEWA MOTOR POS • v0.1.0</Text>
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  profile: {
    backgroundColor: colors.primary,
    borderColor: colors.primary,
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.md,
  },
  avatar: {
    width: 64,
    height: 64,
    borderRadius: radius.lg,
    backgroundColor: colors.onPrimary,
    alignItems: "center",
    justifyContent: "center",
  },
  avatarText: {
    fontFamily: typography.heading,
    fontSize: 20,
    color: colors.primary,
  },
  profileCopy: { flex: 1, gap: 2 },
  name: {
    fontFamily: typography.heading,
    color: colors.onPrimary,
    fontSize: 19,
  },
  username: { ...textStyles.body, color: colors.primarySoft },
  role: {
    marginTop: spacing.xs,
    backgroundColor: colors.onPrimary,
    borderRadius: radius.pill,
    paddingHorizontal: spacing.sm,
    paddingVertical: 4,
    alignSelf: "flex-start",
  },
  roleText: { ...textStyles.label, color: colors.primary, fontSize: 10 },
  section: textStyles.label,
  error: { ...textStyles.body, color: colors.error },
  version: { ...textStyles.label, textAlign: "center", marginTop: spacing.lg },
});
