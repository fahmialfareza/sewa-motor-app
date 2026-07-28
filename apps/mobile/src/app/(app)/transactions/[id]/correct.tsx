import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { StyleSheet, Text, View } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field } from "@/components/ui/Field";
import { QuantityStepper } from "@/components/ui/QuantityStepper";
import { StateView } from "@/components/ui/StateView";
import { correctTransaction, getTransaction } from "@/db/repositories";
import {
  canCorrectTransaction,
  CORRECTION_FORBIDDEN_MESSAGE,
} from "@/domain/permissions";
import type { Transaction } from "@/domain/types";
import { useSyncRuntime } from "@/sync/SyncProvider";
import { colors, spacing, textStyles, typography } from "@/theme/tokens";
import { formatRupiah } from "@/utils/format";

interface TransactionLoadResult {
  id: string;
  transaction: Transaction | null;
  error: string | null;
}

export default function CorrectTransactionScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { session } = useAuth();
  const sync = useSyncRuntime();
  const [loadResult, setLoadResult] = useState<TransactionLoadResult | null>(
    null,
  );
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  const [reason, setReason] = useState("");
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    let active = true;

    if (!id) return;

    void getTransaction(id)
      .then((value) => {
        if (!active) return;

        setLoadResult({ id, transaction: value, error: null });
        if (value) {
          setQuantities(
            Object.fromEntries(
              value.items.map((item) => [item.packageId, item.quantity]),
            ),
          );
        }
      })
      .catch((reasonValue: unknown) => {
        if (!active) return;
        setLoadResult({
          id,
          transaction: null,
          error:
            reasonValue instanceof Error
              ? reasonValue.message
              : "Transaksi tidak dapat dimuat.",
        });
      });

    return () => {
      active = false;
    };
  }, [id]);

  const currentLoad = loadResult?.id === id ? loadResult : null;

  if (id && !currentLoad) {
    return (
      <AppScreen>
        <PageHeader back title="Koreksi transaksi" />
      </AppScreen>
    );
  }

  const transaction = currentLoad?.transaction ?? null;

  if (!transaction) {
    return (
      <AppScreen>
        <PageHeader back title="Koreksi transaksi" />
        <StateView
          actionLabel="Kembali"
          icon="receipt-text-remove-outline"
          message={
            currentLoad?.error ?? "Transaksi tidak ditemukan di perangkat ini."
          }
          onAction={() => router.back()}
          title="Transaksi tidak ditemukan"
        />
      </AppScreen>
    );
  }

  const mayCorrect = canCorrectTransaction(session, transaction);

  if (!mayCorrect) {
    return (
      <AppScreen>
        <PageHeader back title="Koreksi transaksi" />
        <StateView
          actionLabel="Kembali"
          icon="shield-lock-outline"
          message={CORRECTION_FORBIDDEN_MESSAGE}
          onAction={() => router.back()}
          title="Koreksi tidak diizinkan"
        />
      </AppScreen>
    );
  }

  const total = transaction.items.reduce(
    (sum, item) =>
      sum + item.unitPrice * (quantities[item.packageId] ?? item.quantity),
    0,
  );

  const save = async () => {
    if (!session || !canCorrectTransaction(session, transaction)) {
      setError(CORRECTION_FORBIDDEN_MESSAGE);
      return;
    }
    setSaving(true);
    setError(null);
    try {
      await correctTransaction(transaction.id, quantities, reason, session);
      await sync.refresh();
      void sync.syncNow();
      router.replace({
        pathname: "/transactions/[id]",
        params: { id: transaction.id },
      });
    } catch (reasonValue) {
      setError(
        reasonValue instanceof Error
          ? reasonValue.message
          : "Koreksi tidak dapat disimpan.",
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppScreen
      stickyFooter={
        <Button
          icon="content-save-edit-outline"
          loading={saving}
          onPress={() => void save()}
        >
          Simpan koreksi
        </Button>
      }
    >
      <PageHeader
        back
        subtitle={`Revisi saat ini #${transaction.revision}`}
        title="Koreksi Transaksi"
      />
      <Card style={styles.notice}>
        <Text style={styles.noticeTitle}>Revisi tidak menghapus riwayat</Text>
        <Text style={styles.muted}>
          Nilai sebelum dan sesudah disimpan permanen. Struk yang sudah dicetak
          akan ditandai perlu cetak ulang.
        </Text>
      </Card>
      {transaction.items.map((item) => (
        <Card key={item.packageId} style={styles.line}>
          <View style={styles.copy}>
            <Text style={styles.name}>{item.name}</Text>
            <Text style={styles.muted}>{formatRupiah(item.unitPrice)}</Text>
          </View>
          <QuantityStepper
            onChange={(quantity) =>
              setQuantities((current) => ({
                ...current,
                [item.packageId]: quantity,
              }))
            }
            value={quantities[item.packageId] ?? item.quantity}
          />
        </Card>
      ))}
      <Field
        error={
          reason.length > 0 && reason.trim().length < 5
            ? "Alasan minimal 5 karakter."
            : undefined
        }
        label="Alasan koreksi"
        multiline
        numberOfLines={3}
        onChangeText={setReason}
        placeholder="Contoh: jumlah paket salah input"
        style={styles.reason}
        value={reason}
      />
      <Card style={styles.total}>
        <Text style={textStyles.heading}>Total setelah koreksi</Text>
        <Text style={textStyles.price}>{formatRupiah(total)}</Text>
      </Card>
      {error ? <Text style={styles.error}>{error}</Text> : null}
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  notice: { gap: spacing.xs, backgroundColor: colors.primarySoft },
  noticeTitle: { ...textStyles.heading, color: colors.primary },
  muted: { ...textStyles.body, color: colors.textMuted },
  line: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  copy: { flex: 1 },
  name: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    color: colors.text,
  },
  reason: { minHeight: 96, textAlignVertical: "top", paddingTop: spacing.md },
  total: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  error: { ...textStyles.body, color: colors.error },
});
