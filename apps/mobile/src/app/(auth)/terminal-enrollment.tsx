import { useRouter } from "expo-router";
import { useState } from "react";
import { StyleSheet, Text } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field } from "@/components/ui/Field";
import { colors, spacing, textStyles } from "@/theme/tokens";

export default function TerminalEnrollmentScreen() {
  const router = useRouter();
  const { enrollTerminal } = useAuth();
  const [label, setLabel] = useState("MPOS Utama");
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState<string | null>(null);

  const submit = async () => {
    if (label.trim().length < 3) {
      setError("Nama terminal minimal 3 karakter.");
      return;
    }
    setSubmitting(true);
    setError(null);
    try {
      await enrollTerminal(label);
      router.replace("/");
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Pendaftaran terminal gagal.",
      );
    } finally {
      setSubmitting(false);
    }
  };

  return (
    <AppScreen authenticated={false} contentStyle={styles.screen}>
      <Text style={textStyles.title}>Daftarkan terminal</Text>
      <Text style={styles.subtitle}>
        Terminal membuat pasangan kunci Ed25519 yang tersimpan aman di
        perangkat. Pendaftaran pertama memerlukan internet.
      </Text>
      <Card style={styles.card}>
        <Field
          error={error ?? undefined}
          label="Nama terminal"
          onChangeText={setLabel}
          value={label}
        />
        <Button
          icon="shield-key-outline"
          loading={submitting}
          onPress={() => void submit()}
        >
          Daftarkan terminal
        </Button>
      </Card>
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  screen: { flexGrow: 1, justifyContent: "center" },
  subtitle: { ...textStyles.body, color: colors.textMuted },
  card: { gap: spacing.md },
});
