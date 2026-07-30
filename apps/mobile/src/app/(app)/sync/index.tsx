import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import { Alert, StyleSheet, Text, View } from "react-native";

import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { StatusBadge } from "@/components/ui/StatusBadge";
import {
  discardRejectedOutboxOperation,
  getSyncMetadata,
  listConflicts,
  listRejectedOutboxOperations,
  type RejectedOutboxOperation,
} from "@/db/repositories";
import type { SyncConflict } from "@/domain/types";
import { useSyncRuntime } from "@/sync/SyncProvider";
import { colors, spacing, textStyles, typography } from "@/theme/tokens";
import { toUserFacingErrorMessage } from "@/utils/errors";
import { displayTransactionId, formatJakartaDateTime } from "@/utils/format";

export default function SyncCenterScreen() {
  const router = useRouter();
  const runtime = useSyncRuntime();
  const refreshRuntime = runtime.refresh;
  const requestSync = runtime.syncNow;
  const [conflicts, setConflicts] = useState<SyncConflict[]>([]);
  const [rejected, setRejected] = useState<RejectedOutboxOperation[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);
  const [recoveryError, setRecoveryError] = useState<string | null>(null);
  const lastErrorMessage = runtime.lastError
    ? toUserFacingErrorMessage(
        runtime.lastError,
        "Sinkronisasi belum berhasil. Coba lagi.",
      )
    : null;

  const load = useCallback(async () => {
    const [nextConflicts, nextRejected, metadata] = await Promise.all([
      listConflicts(),
      listRejectedOutboxOperations(),
      getSyncMetadata(),
    ]);
    setConflicts(nextConflicts);
    setRejected(nextRejected);
    setCursor(metadata.cursor);
    await refreshRuntime();
  }, [refreshRuntime]);

  useFocusEffect(
    useCallback(() => {
      void load().catch((reason: unknown) => {
        setRecoveryError(
          toUserFacingErrorMessage(
            reason,
            "Status sinkronisasi belum dapat dimuat. Coba lagi.",
          ),
        );
      });
    }, [load]),
  );

  const syncNow = useCallback(async () => {
    try {
      await requestSync();
    } catch {
      // The sync store exposes the translated error in runtime.lastError.
    }
    try {
      await load();
      setRecoveryError(null);
    } catch (reason) {
      setRecoveryError(
        toUserFacingErrorMessage(
          reason,
          "Status sinkronisasi belum dapat dimuat. Coba lagi.",
        ),
      );
    }
  }, [load, requestSync]);

  const recoverRejected = (operation: RejectedOutboxOperation) => {
    const isRejectedCreate =
      operation.aggregate === "transaction" && operation.action === "create";
    const title = isRejectedCreate
      ? "Arsipkan transaksi lokal?"
      : "Pulihkan kondisi sebelum operasi?";
    const message = isRejectedCreate
      ? "Transaksi ini tidak pernah diterima server. Salinan lokal hanya dapat diarsipkan jika belum dibayar dan belum pernah dicetak. Catatan yang sudah dibayar atau dicetak tidak akan diarsipkan dan memerlukan rekonsiliasi dengan server."
      : "Perubahan optimistis akan dibatalkan ke kondisi aman terakhir, lalu operasi dikeluarkan dari antrean.";

    Alert.alert(title, message, [
      { text: "Batal", style: "cancel" },
      {
        text: isRejectedCreate ? "Arsipkan" : "Pulihkan",
        style: isRejectedCreate ? "destructive" : "default",
        onPress: () => {
          setRecoveryError(null);
          void discardRejectedOutboxOperation(operation.operationId)
            .then(load)
            .catch((reason: unknown) => {
              setRecoveryError(
                reason instanceof Error
                  ? reason.message
                  : "Operasi lokal tidak dapat dipulihkan.",
              );
            });
        },
      },
    ]);
  };

  return (
    <AppScreen>
      <PageHeader
        back
        subtitle="Pemulihan data lokal-first"
        title="Pusat Sinkron"
      />
      <Card style={styles.hero}>
        <View style={styles.heroTop}>
          <StatusBadge kind={runtime.online ? "online" : "offline"} />
          <Text style={styles.pending}>
            {runtime.pendingCount} operasi menunggu
          </Text>
        </View>
        <Text style={styles.detail}>
          {runtime.lastSyncedAt
            ? `Terakhir berhasil ${formatJakartaDateTime(runtime.lastSyncedAt)}`
            : "Belum pernah sinkron pada instalasi ini."}
        </Text>
        <Text numberOfLines={1} style={styles.cursor}>
          CURSOR {cursor ?? "AWAL"}
        </Text>
        <Button
          disabled={!runtime.online}
          icon="sync"
          loading={runtime.syncing}
          onPress={() => void syncNow()}
        >
          Sinkron sekarang
        </Button>
      </Card>
      {lastErrorMessage ? (
        <Card style={styles.errorCard}>
          <Text accessibilityRole="alert" style={styles.errorTitle}>
            Sinkronisasi belum berhasil
          </Text>
          <Text style={styles.detail}>{lastErrorMessage}</Text>
          <Text style={styles.errorHint}>
            Data lokal tetap tersimpan dan sinkronisasi otomatis akan mencoba
            lagi.
          </Text>
        </Card>
      ) : null}
      {recoveryError ? (
        <Text accessibilityRole="alert" style={styles.recoveryError}>
          {recoveryError}
        </Text>
      ) : null}
      {rejected.length > 0 ? (
        <>
          <Text style={textStyles.heading}>Operasi ditolak server</Text>
          {rejected.map((operation) => (
            <Card key={operation.operationId} style={styles.errorCard}>
              <Text style={styles.errorTitle}>{operation.aggregateId}</Text>
              <Text style={styles.detail}>{operation.message}</Text>
              <Button
                onPress={() => recoverRejected(operation)}
                variant="danger"
              >
                Pulihkan dan keluarkan
              </Button>
            </Card>
          ))}
        </>
      ) : null}
      <Text style={textStyles.heading}>Konflik yang perlu ditinjau</Text>
      {conflicts.length === 0 ? (
        <Card>
          <Text style={styles.detail}>
            Tidak ada konflik. Operasi pending akan dikirim FIFO saat koneksi
            tersedia.
          </Text>
        </Card>
      ) : (
        conflicts.map((conflict) => (
          <Card key={conflict.id} style={styles.conflict}>
            <View style={styles.conflictCopy}>
              <Text style={styles.conflictId}>
                {displayTransactionId(conflict.transactionId)}
              </Text>
              <Text style={styles.detail}>
                Lokal rev. {conflict.localSnapshot.revision} • Server rev.{" "}
                {conflict.serverSnapshot.revision}
              </Text>
            </View>
            <Button
              onPress={() =>
                router.push({
                  pathname: "/transactions/[id]/conflict",
                  params: { id: conflict.transactionId },
                })
              }
              variant="secondary"
            >
              Tinjau
            </Button>
          </Card>
        ))
      )}
      <Card style={styles.note}>
        <Text style={textStyles.label}>CARA KERJA</Text>
        <Text style={styles.detail}>
          Sinkron dipicu setelah login, perubahan lokal, koneksi pulih, aplikasi
          aktif kembali, tombol manual, dan background task. Background task
          bersifat best-effort; indikator ini adalah sumber pemulihan utama.
        </Text>
      </Card>
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  hero: { gap: spacing.md },
  heroTop: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  pending: {
    fontFamily: typography.heading,
    fontSize: 17,
    color: colors.text,
  },
  detail: { ...textStyles.body, color: colors.textMuted },
  cursor: { ...textStyles.technical, fontSize: 9 },
  errorCard: { gap: spacing.xs, backgroundColor: colors.errorSoft },
  errorTitle: { ...textStyles.heading, color: colors.error },
  errorHint: { ...textStyles.body, color: colors.textMuted, fontSize: 12 },
  recoveryError: { ...textStyles.body, color: colors.error },
  conflict: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  conflictCopy: { flex: 1 },
  conflictId: { ...textStyles.technical, color: colors.primary },
  note: { gap: spacing.sm, backgroundColor: colors.surfaceBright },
});
