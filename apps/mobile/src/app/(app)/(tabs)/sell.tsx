import { useFocusEffect, useRouter } from "expo-router";
import { memo, useCallback, useMemo, useState } from "react";
import {
  ActivityIndicator,
  FlatList,
  ScrollView,
  StyleSheet,
  Text,
  View,
  type ListRenderItemInfo,
} from "react-native";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { PaymentMethodSelector } from "@/components/transactions/PaymentMethodSelector";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Icon } from "@/components/ui/Icon";
import { QuantityStepper } from "@/components/ui/QuantityStepper";
import { StateView } from "@/components/ui/StateView";
import { createTransaction, listPackages } from "@/db/repositories";
import {
  createDynamicQris,
  fingerprintStaticQris,
  validateStaticQris,
} from "@/domain/qris";
import type {
  QrisPayloadHash,
  RentalPackage,
  SelectablePaymentMethod,
} from "@/domain/types";
import { readQrisConfig } from "@/security/secure-store";
import { useSyncRuntime } from "@/sync/SyncProvider";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";
import { formatRupiah } from "@/utils/format";

export default function SaleComposerScreen() {
  const router = useRouter();
  const { session } = useAuth();
  const sync = useSyncRuntime();
  const [packages, setPackages] = useState<RentalPackage[]>([]);
  const [quantities, setQuantities] = useState<Record<string, number>>({});
  const [paymentMethod, setPaymentMethod] =
    useState<SelectablePaymentMethod | null>(null);
  const [loadingPackages, setLoadingPackages] = useState(true);
  const [packageError, setPackageError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const [qrisAvailable, setQrisAvailable] = useState(false);
  const [qrisConfigChecked, setQrisConfigChecked] = useState(false);

  const loadPackages = useCallback(async () => {
    setLoadingPackages(true);
    setPackageError(null);
    try {
      setPackages(await listPackages());
    } catch (reason) {
      setPackageError(
        reason instanceof Error
          ? reason.message
          : "Daftar paket tidak dapat dimuat.",
      );
    } finally {
      setLoadingPackages(false);
    }
  }, []);

  const loadQrisAvailability = useCallback(async () => {
    try {
      const config = await readQrisConfig();
      const available =
        config !== null && Boolean(validateStaticQris(config.staticPayload));
      setQrisAvailable(available);
      if (!available) {
        setPaymentMethod((current) => (current === "qris" ? null : current));
      }
    } catch {
      setQrisAvailable(false);
      setPaymentMethod((current) => (current === "qris" ? null : current));
    } finally {
      setQrisConfigChecked(true);
    }
  }, []);

  useFocusEffect(
    useCallback(() => {
      void Promise.all([loadPackages(), loadQrisAvailability()]);
    }, [loadPackages, loadQrisAvailability]),
  );

  const total = useMemo(
    () =>
      packages.reduce(
        (sum, item) => sum + item.unitPrice * (quantities[item.id] ?? 0),
        0,
      ),
    [packages, quantities],
  );

  const selectedPackages = useMemo(
    () => packages.filter((item) => (quantities[item.id] ?? 0) > 0),
    [packages, quantities],
  );

  const selectedItemCount = useMemo(
    () =>
      selectedPackages.reduce(
        (sum, item) => sum + (quantities[item.id] ?? 0),
        0,
      ),
    [quantities, selectedPackages],
  );

  const updateQuantity = useCallback((packageId: string, quantity: number) => {
    setQuantities((current) => ({ ...current, [packageId]: quantity }));
  }, []);

  const renderPackage = useCallback(
    ({ item }: ListRenderItemInfo<RentalPackage>) => (
      <PackageSelectorCard
        item={item}
        onChange={updateQuantity}
        quantity={quantities[item.id] ?? 0}
      />
    ),
    [quantities, updateQuantity],
  );

  const save = async () => {
    if (!session) return;
    if (!paymentMethod) {
      setError("Pilih metode pembayaran tunai atau QRIS.");
      return;
    }
    setSaving(true);
    setError(null);
    try {
      let qrisPayloadHash: QrisPayloadHash | null = null;
      if (paymentMethod === "qris") {
        const qrisConfig = await readQrisConfig();
        if (!qrisConfig) {
          throw new Error(
            "QRIS belum dikonfigurasi. Minta superadmin mengatur QRIS merchant.",
          );
        }
        const staticQris = validateStaticQris(qrisConfig.staticPayload);
        createDynamicQris(staticQris.payload, total);
        qrisPayloadHash = await fingerprintStaticQris(staticQris.payload);
      }
      const transaction = await createTransaction(
        packages.map((item) => ({
          package: item,
          quantity: quantities[item.id] ?? 0,
        })),
        paymentMethod,
        qrisPayloadHash,
        session,
      );
      setQuantities({});
      setPaymentMethod(null);
      await sync.refresh();
      void sync.syncNow();
      router.push({
        pathname: "/transactions/[id]",
        params: { id: transaction.id },
      });
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Transaksi tidak dapat disimpan.",
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <AppScreen
      contentStyle={styles.screen}
      scroll={false}
      stickyFooter={
        <StickyTransactionSummary
          disabled={total === 0 || !paymentMethod}
          error={error}
          itemCount={selectedItemCount}
          loading={saving}
          onSave={() => void save()}
          onPaymentMethodChange={setPaymentMethod}
          packageCount={selectedPackages.length}
          paymentMethod={paymentMethod}
          qrisAvailable={qrisAvailable}
          qrisConfigChecked={qrisConfigChecked}
          quantities={quantities}
          selectedPackages={selectedPackages}
          total={total}
        />
      }
    >
      <FlatList
        contentContainerStyle={styles.listContent}
        data={packages}
        initialNumToRender={6}
        ItemSeparatorComponent={PackageSeparator}
        keyExtractor={(item) => item.id}
        keyboardShouldPersistTaps="handled"
        ListEmptyComponent={
          loadingPackages ? (
            <View style={styles.loading}>
              <ActivityIndicator color={colors.primary} />
              <Text style={styles.loadingText}>Memuat paket aktif…</Text>
            </View>
          ) : (
            <StateView
              {...(packageError
                ? {
                    actionLabel: "Coba lagi",
                    onAction: () => void loadPackages(),
                  }
                : {})}
              icon={packageError ? "alert-circle-outline" : "package-variant"}
              message={
                packageError ??
                "Belum ada paket aktif. Superadmin dapat menambahkannya dari Pengaturan."
              }
              title={packageError ? "Paket gagal dimuat" : "Belum ada paket"}
            />
          )
        }
        ListHeaderComponent={
          <View style={styles.listHeader}>
            <PageHeader
              subtitle={`Kasir • ${session?.user.fullName ?? "-"}`}
              title="Transaksi baru"
            />
            <View style={styles.guidance}>
              <View style={styles.guidanceIcon}>
                <Icon color={colors.primary} name="information-outline" />
              </View>
              <Text style={styles.instruction}>
                Tentukan jumlah pada paket yang dipilih. Harga tersimpan sebagai
                snapshot transaksi.
              </Text>
            </View>
            <View style={styles.sectionHeader}>
              <View>
                <Text style={styles.sectionEyebrow}>KATALOG AKTIF</Text>
                <Text style={styles.sectionTitle}>Pilih paket</Text>
              </View>
              <View style={styles.packageCount}>
                <Text style={styles.packageCountText}>{packages.length}</Text>
              </View>
            </View>
            {packageError && packages.length > 0 ? (
              <Text accessibilityRole="alert" style={styles.error}>
                {packageError}
              </Text>
            ) : null}
          </View>
        }
        onRefresh={() => void loadPackages()}
        refreshing={loadingPackages && packages.length > 0}
        renderItem={renderPackage}
        showsVerticalScrollIndicator={false}
        style={styles.list}
      />
    </AppScreen>
  );
}

const PackageSelectorCard = memo(function PackageSelectorCard({
  item,
  quantity,
  onChange,
}: {
  item: RentalPackage;
  quantity: number;
  onChange: (packageId: string, quantity: number) => void;
}) {
  const accent =
    item.accent === "sunrise"
      ? colors.sunrise
      : item.accent === "standard"
        ? colors.standard
        : colors.primary;
  const label =
    item.accent === "sunrise"
      ? "SUNRISE"
      : item.accent === "standard"
        ? "STANDAR"
        : "PAKET";

  return (
    <Card
      style={[
        styles.packageCard,
        {
          borderColor: quantity > 0 ? accent : colors.outline,
          borderLeftColor: accent,
        },
      ]}
    >
      <View style={styles.packageTop}>
        <View style={styles.packageCopy}>
          <Text style={[styles.packageLabel, { color: accent }]}>{label}</Text>
          <Text style={styles.packageName}>{item.name}</Text>
        </View>
        {quantity > 0 ? (
          <View style={[styles.selectedBadge, { backgroundColor: accent }]}>
            <Icon color={colors.onPrimary} name="check" size={16} />
          </View>
        ) : null}
      </View>
      <Text numberOfLines={2} style={styles.description}>
        {item.description}
      </Text>
      <View style={styles.packageBottom}>
        <View>
          <Text style={styles.priceLabel}>HARGA SATUAN</Text>
          <Text style={[styles.packagePrice, { color: accent }]}>
            {formatRupiah(item.unitPrice)}
          </Text>
        </View>
        <QuantityStepper
          onChange={(nextQuantity) => onChange(item.id, nextQuantity)}
          value={quantity}
        />
      </View>
    </Card>
  );
});

function PackageSeparator() {
  return <View style={styles.packageSeparator} />;
}

function StickyTransactionSummary({
  packageCount,
  itemCount,
  total,
  disabled,
  loading,
  error,
  onSave,
  paymentMethod,
  qrisAvailable,
  qrisConfigChecked,
  onPaymentMethodChange,
  selectedPackages,
  quantities,
}: {
  packageCount: number;
  itemCount: number;
  total: number;
  disabled: boolean;
  loading: boolean;
  error: string | null;
  onSave: () => void;
  paymentMethod: SelectablePaymentMethod | null;
  qrisAvailable: boolean;
  qrisConfigChecked: boolean;
  onPaymentMethodChange: (method: SelectablePaymentMethod) => void;
  selectedPackages: RentalPackage[];
  quantities: Record<string, number>;
}) {
  return (
    <Card style={styles.stickySummary}>
      <View style={styles.stickySummaryHeader}>
        <View>
          <Text style={styles.sectionEyebrow}>RINGKASAN</Text>
          <Text style={styles.stickySummaryMeta}>
            {packageCount === 0
              ? "Belum ada paket dipilih"
              : `${packageCount} paket • ${itemCount} item`}
          </Text>
        </View>
      </View>
      {selectedPackages.length > 0 ? (
        <ScrollView
          contentContainerStyle={styles.summaryLinesContent}
          nestedScrollEnabled
          showsVerticalScrollIndicator={selectedPackages.length > 2}
          style={styles.summaryLines}
        >
          {selectedPackages.map((item) => {
            const quantity = quantities[item.id] ?? 0;

            return (
              <View key={item.id} style={styles.summaryLine}>
                <View style={styles.summaryLineCopy}>
                  <Text numberOfLines={1} style={styles.summaryLineName}>
                    {item.name}
                  </Text>
                  <Text style={styles.summaryLineCalculation}>
                    {quantity} × {formatRupiah(item.unitPrice)}
                  </Text>
                </View>
                <Text style={styles.summaryLineTotal}>
                  {formatRupiah(quantity * item.unitPrice)}
                </Text>
              </View>
            );
          })}
        </ScrollView>
      ) : null}
      <View style={styles.stickyTotalRow}>
        <Text style={styles.stickyTotalLabel}>Total pembayaran</Text>
        <Text accessibilityLiveRegion="polite" style={styles.stickyTotal}>
          {formatRupiah(total)}
        </Text>
      </View>
      <PaymentMethodSelector
        onChange={onPaymentMethodChange}
        qrisDisabled={!qrisConfigChecked || !qrisAvailable}
        qrisDisabledReason={
          qrisConfigChecked
            ? "QRIS belum dikonfigurasi oleh superadmin."
            : "Memeriksa konfigurasi QRIS…"
        }
        value={paymentMethod}
      />
      {error ? (
        <Text accessibilityRole="alert" style={styles.stickyError}>
          {error}
        </Text>
      ) : null}
      <Button
        disabled={disabled}
        icon="content-save-check-outline"
        loading={loading}
        onPress={onSave}
      >
        Simpan transaksi
      </Button>
    </Card>
  );
}

const styles = StyleSheet.create({
  screen: {
    padding: 0,
    paddingBottom: 0,
    gap: 0,
  },
  list: { flex: 1 },
  listContent: {
    paddingHorizontal: spacing.md,
    paddingTop: spacing.md,
    paddingBottom: spacing.xl,
  },
  listHeader: {
    gap: spacing.md,
    marginBottom: spacing.md,
  },
  guidance: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
    padding: spacing.md,
    borderRadius: radius.lg,
    backgroundColor: colors.primarySoft,
  },
  guidanceIcon: {
    width: 36,
    height: 36,
    borderRadius: radius.pill,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.card,
  },
  instruction: { ...textStyles.body, color: colors.textMuted, flex: 1 },
  sectionHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
  },
  sectionEyebrow: {
    ...textStyles.label,
    color: colors.primary,
    fontSize: 10,
  },
  sectionTitle: { ...textStyles.heading, marginTop: 2 },
  packageCount: {
    minWidth: 36,
    height: 36,
    paddingHorizontal: spacing.sm,
    borderRadius: radius.pill,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.container,
  },
  packageCountText: {
    ...textStyles.body,
    fontFamily: typography.bodySemibold,
    color: colors.primary,
  },
  packageCard: {
    borderLeftWidth: 5,
    gap: spacing.sm,
  },
  packageTop: {
    flexDirection: "row",
    alignItems: "flex-start",
    gap: spacing.sm,
  },
  packageCopy: { flex: 1, gap: 2 },
  packageLabel: { ...textStyles.label, fontSize: 10 },
  packageName: {
    fontFamily: typography.heading,
    fontSize: 18,
    color: colors.text,
  },
  description: { ...textStyles.body, color: colors.textMuted, fontSize: 13 },
  selectedBadge: {
    width: 28,
    height: 28,
    borderRadius: radius.pill,
    alignItems: "center",
    justifyContent: "center",
  },
  packageBottom: {
    flexDirection: "row",
    alignItems: "flex-end",
    justifyContent: "space-between",
    gap: spacing.sm,
    paddingTop: spacing.sm,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: colors.outline,
  },
  priceLabel: { ...textStyles.label, fontSize: 9 },
  packagePrice: {
    fontFamily: typography.heading,
    fontSize: 18,
    marginTop: 2,
  },
  packageSeparator: { height: spacing.sm },
  stickySummary: {
    gap: spacing.sm,
    backgroundColor: colors.surfaceBright,
  },
  stickySummaryHeader: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: spacing.sm,
  },
  stickySummaryMeta: {
    ...textStyles.body,
    color: colors.textMuted,
    fontFamily: typography.bodyMedium,
    marginTop: 2,
  },
  summaryLines: {
    maxHeight: 112,
    paddingRight: spacing.xs,
  },
  summaryLinesContent: {
    gap: spacing.sm,
  },
  summaryLine: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
  },
  summaryLineCopy: {
    flex: 1,
  },
  summaryLineName: {
    ...textStyles.body,
    color: colors.text,
    fontFamily: typography.bodySemibold,
  },
  summaryLineCalculation: {
    ...textStyles.label,
    color: colors.textMuted,
    fontSize: 10,
  },
  summaryLineTotal: {
    ...textStyles.body,
    color: colors.text,
    fontFamily: typography.bodySemibold,
    textAlign: "right",
  },
  stickyTotalRow: {
    flexDirection: "row",
    alignItems: "center",
    justifyContent: "space-between",
    gap: spacing.sm,
    paddingTop: spacing.sm,
    borderTopWidth: StyleSheet.hairlineWidth,
    borderTopColor: colors.outline,
  },
  stickyTotalLabel: {
    ...textStyles.body,
    color: colors.text,
    fontFamily: typography.bodySemibold,
  },
  stickyTotal: {
    fontFamily: typography.price,
    fontSize: 22,
    lineHeight: 28,
    letterSpacing: -0.4,
    color: colors.primary,
    textAlign: "right",
  },
  error: {
    ...textStyles.body,
    color: colors.error,
    backgroundColor: colors.errorSoft,
    padding: spacing.md,
    borderRadius: radius.md,
  },
  stickyError: {
    ...textStyles.body,
    color: colors.error,
    fontSize: 12,
  },
  loading: {
    paddingVertical: spacing.xl,
    alignItems: "center",
    gap: spacing.sm,
  },
  loadingText: { ...textStyles.body, color: colors.textMuted },
});
