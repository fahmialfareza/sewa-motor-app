import { useLocalSearchParams, useRouter } from "expo-router";
import { useEffect, useState } from "react";
import { StyleSheet, Text, View } from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { PaymentMethodBadge } from "@/components/ui/PaymentBadge";
import { StateView } from "@/components/ui/StateView";
import {
  beginPrintAttempt,
  completePrintAttempt,
  getTransaction,
} from "@/db/repositories";
import type { Transaction } from "@/domain/types";
import { isPaymentConfirmedForCurrentRevision } from "@/domain/payments";
import { getConfiguredPrinter } from "@/printer/service";
import { receiptFromTransaction } from "@/printer/types";
import { useSyncRuntime } from "@/sync/SyncProvider";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";
import { displayTransactionId, formatRupiah } from "@/utils/format";

export default function PrintTransactionScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  const router = useRouter();
  const { session } = useAuth();
  const sync = useSyncRuntime();
  const [transaction, setTransaction] = useState<Transaction | null>(null);
  const [printing, setPrinting] = useState(false);
  const [success, setSuccess] = useState(false);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    if (id && session) {
      void getTransaction(id).then(setTransaction);
    }
  }, [id, session]);

  if (!transaction || !session) return <AppScreen />;
  if (!isPaymentConfirmedForCurrentRevision(transaction)) {
    return (
      <AppScreen>
        <PageHeader back title="Cetak struk" />
        <StateView
          actionLabel="Buka detail transaksi"
          icon="lock-outline"
          message="Pembayaran harus berhasil untuk revisi transaksi saat ini sebelum struk dapat dicetak."
          onAction={() =>
            router.replace({
              pathname: "/transactions/[id]",
              params: { id: transaction.id },
            })
          }
          title="Pencetakan terkunci"
        />
      </AppScreen>
    );
  }
  const isCopy =
    transaction.printState === "success" ||
    transaction.printState === "unknown" ||
    transaction.printState === "needs-reprint";

  const print = async (forceCopy?: boolean) => {
    const printAsCopy = forceCopy ?? isCopy;
    setPrinting(true);
    setError(null);
    let attemptId: string | null = null;
    try {
      const { config, printer } = await getConfiguredPrinter();
      attemptId = await beginPrintAttempt({
        transactionId: transaction.id,
        transactionRevision: transaction.revision,
        adapter: config.adapter,
        isCopy: printAsCopy,
        session,
      });
      await printer.connect(config.address ?? undefined);
      const result = await printer.print(
        receiptFromTransaction(transaction, printAsCopy),
      );
      await printer.disconnect().catch(() => undefined);
      await completePrintAttempt({
        attemptId,
        transactionId: transaction.id,
        result: result.status,
        ...(result.status === "success" ? {} : { error: result.message }),
        session,
      });
      await sync.refresh();
      void sync.syncNow();
      if (result.status === "success") {
        setTransaction((current) =>
          current ? { ...current, printState: "success" } : current,
        );
        setSuccess(true);
      } else {
        router.replace({
          pathname: "/transactions/[id]/print-failure",
          params: {
            id: transaction.id,
            status: result.status,
            message: result.message,
          },
        });
      }
    } catch (reason) {
      const message =
        reason instanceof Error ? reason.message : "Printer gagal digunakan.";
      if (attemptId) {
        await completePrintAttempt({
          attemptId,
          transactionId: transaction.id,
          result: "failed",
          error: message,
          session,
        }).catch(() => undefined);
      }
      setError(message);
    } finally {
      setPrinting(false);
    }
  };

  return (
    <AppScreen>
      <PageHeader back title={success ? "Struk tercetak" : "Cetak struk"} />
      <View style={[styles.icon, success && styles.iconSuccess]}>
        <Text style={styles.iconGlyph}>{success ? "✓" : "✓"}</Text>
      </View>
      <Text style={styles.title}>
        {success ? "Struk berhasil dicetak!" : "Pembayaran berhasil"}
      </Text>
      <Text style={styles.subtitle}>
        {success
          ? "Penjualan tetap tercatat dan hasil cetak masuk ke antrean sinkron."
          : "Pembayaran sudah dikonfirmasi untuk revisi transaksi ini. Struk siap dicetak."}
      </Text>
      <Card style={styles.summary}>
        <View style={styles.summaryHeader}>
          <Text style={styles.id}>{displayTransactionId(transaction.id)}</Text>
          <PaymentMethodBadge method={transaction.paymentMethod} />
        </View>
        {transaction.items.map((item) => (
          <View key={item.id} style={styles.line}>
            <Text style={styles.lineName}>
              {item.quantity} × {item.name}
            </Text>
            <Text style={styles.lineValue}>{formatRupiah(item.lineTotal)}</Text>
          </View>
        ))}
        <View style={styles.total}>
          <Text style={textStyles.heading}>Total</Text>
          <Text style={textStyles.price}>
            {formatRupiah(transaction.total)}
          </Text>
        </View>
      </Card>
      {error ? <Text style={styles.error}>{error}</Text> : null}
      {success ? (
        <>
          <Button
            icon="home-outline"
            onPress={() => router.replace("/(app)/(tabs)/home")}
          >
            Kembali ke Beranda
          </Button>
          <Button
            icon="printer-outline"
            onPress={() => {
              setSuccess(false);
              void print(true);
            }}
            variant="secondary"
          >
            Cetak salinan
          </Button>
        </>
      ) : (
        <>
          <Button
            icon="printer-outline"
            loading={printing}
            onPress={() => void print()}
          >
            {isCopy ? "Cetak salinan" : "Cetak struk"}
          </Button>
          <Button
            disabled={printing}
            onPress={() => router.replace("/(app)/(tabs)/history")}
            variant="ghost"
          >
            Cetak nanti
          </Button>
        </>
      )}
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  icon: {
    width: 84,
    height: 84,
    borderRadius: radius.xl,
    backgroundColor: colors.primary,
    alignSelf: "center",
    alignItems: "center",
    justifyContent: "center",
  },
  iconSuccess: { backgroundColor: colors.success },
  iconGlyph: {
    fontFamily: typography.heading,
    fontSize: 46,
    color: colors.onPrimary,
  },
  title: { ...textStyles.title, textAlign: "center" },
  subtitle: {
    ...textStyles.body,
    color: colors.textMuted,
    textAlign: "center",
  },
  summary: { gap: spacing.sm },
  summaryHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: spacing.sm,
  },
  id: { ...textStyles.label, color: colors.primary },
  line: { flexDirection: "row", justifyContent: "space-between", gap: 8 },
  lineName: { ...textStyles.body, flex: 1 },
  lineValue: { ...textStyles.body, fontFamily: typography.bodySemibold },
  total: {
    marginTop: spacing.sm,
    paddingTop: spacing.md,
    borderTopColor: colors.outline,
    borderTopWidth: 1,
    flexDirection: "row",
    justifyContent: "space-between",
    alignItems: "center",
  },
  error: {
    ...textStyles.body,
    color: colors.error,
    backgroundColor: colors.errorSoft,
    padding: spacing.md,
    borderRadius: radius.md,
  },
});
