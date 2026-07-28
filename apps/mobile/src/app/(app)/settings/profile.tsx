import { StyleSheet, Text, View } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Card } from "@/components/ui/Card";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";
import { initials } from "@/utils/format";

export default function ProfileScreen() {
  const { session } = useAuth();
  if (!session) return <AppScreen />;
  return (
    <AppScreen>
      <PageHeader back title="Profil" />
      <Card style={styles.profile}>
        <View style={styles.avatar}>
          <Text style={styles.avatarText}>
            {initials(session.user.fullName)}
          </Text>
        </View>
        <Text style={styles.name}>{session.user.fullName}</Text>
        <Text style={styles.username}>@{session.user.username}</Text>
      </Card>
      <Card style={styles.details}>
        <Row label="ID pengguna" value={session.user.id} />
        <Row label="Peran" value={session.user.role.toUpperCase()} />
        <Row
          label="Status"
          value={session.user.active ? "AKTIF" : "NONAKTIF"}
        />
        <Row label="ID sesi" value={session.sessionId} />
      </Card>
      <Text style={styles.note}>
        Nama, username, dan peran dikelola oleh superadmin agar jejak audit
        tetap konsisten.
      </Text>
    </AppScreen>
  );
}

function Row({ label, value }: { label: string; value: string }) {
  return (
    <View style={styles.row}>
      <Text style={textStyles.label}>{label.toUpperCase()}</Text>
      <Text selectable style={styles.value}>
        {value}
      </Text>
    </View>
  );
}

const styles = StyleSheet.create({
  profile: { alignItems: "center", gap: spacing.xs },
  avatar: {
    width: 72,
    height: 72,
    borderRadius: radius.lg,
    backgroundColor: colors.primary,
    alignItems: "center",
    justifyContent: "center",
  },
  avatarText: {
    fontFamily: typography.heading,
    fontSize: 22,
    color: colors.onPrimary,
  },
  name: { ...textStyles.heading, marginTop: spacing.sm },
  username: { ...textStyles.body, color: colors.textMuted },
  details: { gap: spacing.md },
  row: { gap: spacing.xs },
  value: { ...textStyles.body, fontFamily: typography.bodyMedium },
  note: { ...textStyles.body, color: colors.textMuted, textAlign: "center" },
});
