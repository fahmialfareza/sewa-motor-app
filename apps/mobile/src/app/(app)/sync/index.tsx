import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import { StyleSheet, Text, View } from "react-native";

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
import { displayTransactionId, formatJakartaDateTime } from "@/utils/format";

export default function SyncCenterScreen() {
  const router = useRouter();
  const runtime = useSyncRuntime();
  const refreshRuntime = runtime.refresh;
  const [conflicts, setConflicts] = useState<SyncConflict[]>([]);
  const [rejected, setRejected] = useState<RejectedOutboxOperation[]>([]);
  const [cursor, setCursor] = useState<string | null>(null);

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
      void load();
    }, [load]),
  );

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
          onPress={() => void runtime.syncNow().then(() => load())}
        >
          Sinkron sekarang
        </Button>
      </Card>
      {runtime.lastError ? (
        <Card style={styles.errorCard}>
          <Text style={styles.errorTitle}>Sinkron terakhir gagal</Text>
          <Text style={styles.detail}>{runtime.lastError}</Text>
        </Card>
      ) : null}
      {rejected.length > 0 ? (
        <>
          <Text style={textStyles.heading}>Operasi ditolak server</Text>
          {rejected.map((operation) => (
            <Card key={operation.operationId} style={styles.errorCard}>
              <Text style={styles.errorTitle}>{operation.aggregateId}</Text>
              <Text style={styles.detail}>{operation.message}</Text>
              <Button
                onPress={() =>
                  void discardRejectedOutboxOperation(
                    operation.operationId,
                  ).then(load)
                }
                variant="danger"
              >
                Akui dan keluarkan dari antrean
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
  cursor: { ...textStyles.label, fontSize: 9 },
  errorCard: { gap: spacing.xs, backgroundColor: colors.errorSoft },
  errorTitle: { ...textStyles.heading, color: colors.error },
  conflict: { flexDirection: "row", alignItems: "center", gap: spacing.sm },
  conflictCopy: { flex: 1 },
  conflictId: { ...textStyles.label, color: colors.primary },
  note: { gap: spacing.sm, backgroundColor: colors.surfaceBright },
});
