import * as ImagePicker from "expo-image-picker";
import { useEffect, useRef, useState } from "react";
import { Alert, StyleSheet, Text, View } from "react-native";
import QRCode from "react-native-qrcode-svg";

import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Icon } from "@/components/ui/Icon";
import { StateView } from "@/components/ui/StateView";
import { readStaticQrisFromImage } from "@/domain/qris-image";
import { validateStaticQris, type ParsedQris } from "@/domain/qris";
import {
  clearQrisConfig,
  readQrisConfig,
  writeQrisConfig,
} from "@/security/secure-store";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";

const MAX_QRIS_IMAGE_BYTES = 25 * 1024 * 1024;
const QRIS_IMAGE_OPTIONS: ImagePicker.ImagePickerOptions = {
  mediaTypes: ["images"],
  allowsEditing: true,
  aspect: [1, 1],
  quality: 1,
};

type PickerSource = "camera" | "gallery";

interface QrisSummaryCardProps {
  qris: ParsedQris;
  status: "candidate" | "saved";
}

function QrisSummaryCard({ qris, status }: QrisSummaryCardProps) {
  const saved = status === "saved";
  return (
    <Card style={saved ? styles.savedCard : styles.candidateCard}>
      <View style={styles.lockHeader}>
        <View
          accessibilityElementsHidden
          importantForAccessibility="no-hide-descendants"
          style={saved ? styles.lockIcon : styles.candidateIcon}
        >
          <Icon
            color={saved ? colors.success : colors.primary}
            name={saved ? "lock-check-outline" : "qrcode-scan"}
            size={24}
          />
        </View>
        <View style={styles.lockCopy}>
          <Text style={textStyles.label}>
            {saved ? "QRIS STATIS TERKUNCI" : "QRIS STATIS TERBACA"}
          </Text>
          <Text style={styles.lockHint}>
            {saved
              ? "Payload tidak dapat diedit manual."
              : "Periksa merchant sebelum menyimpan."}
          </Text>
        </View>
      </View>
      <View
        accessibilityLabel={`QRIS statis ${qris.merchantName}`}
        accessibilityRole="image"
        style={styles.qrFrame}
      >
        <QRCode
          backgroundColor="#FFFFFF"
          color="#000000"
          ecl="M"
          quietZone={24}
          size={184}
          value={qris.payload}
        />
      </View>
      <View style={styles.merchantCopy}>
        <Text style={styles.merchantName}>{qris.merchantName}</Text>
        <Text style={styles.merchantCity}>{qris.merchantCity}</Text>
      </View>
      <View
        accessibilityLabel="Payload QRIS statis hanya dapat dibaca"
        style={styles.payloadReference}
      >
        <Icon color={colors.textMuted} name="code-tags" size={16} />
        <Text numberOfLines={1} style={styles.payloadText}>
          {formatPayloadReference(qris.payload)}
        </Text>
      </View>
    </Card>
  );
}

function formatPayloadReference(payload: string): string {
  return `${payload.slice(0, 20)}…${payload.slice(-12)}`;
}

async function parseQrisImageAsset(
  asset: ImagePicker.ImagePickerAsset,
): Promise<ParsedQris> {
  if (asset.fileSize !== undefined && asset.fileSize > MAX_QRIS_IMAGE_BYTES) {
    throw new Error("Ukuran gambar QRIS maksimal 25 MB.");
  }
  if (
    asset.type !== undefined &&
    asset.type !== null &&
    asset.type !== "image" &&
    asset.type !== "livePhoto"
  ) {
    throw new Error("File yang dipilih harus berupa gambar.");
  }
  return readStaticQrisFromImage(asset.uri);
}

export default function QrisSettingsScreen() {
  const { session } = useAuth();
  const role = session?.user.role;
  const [saved, setSaved] = useState<ParsedQris | null>(null);
  const [candidate, setCandidate] = useState<ParsedQris | null>(null);
  const [replacing, setReplacing] = useState(false);
  const [loading, setLoading] = useState(true);
  const [processingSource, setProcessingSource] = useState<PickerSource | null>(
    null,
  );
  const [saving, setSaving] = useState(false);
  const [message, setMessage] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const pickerActiveRef = useRef(false);
  const saveActiveRef = useRef(false);

  useEffect(() => {
    if (role !== "superadmin") {
      return;
    }

    let active = true;
    void (async () => {
      const config = await readQrisConfig();
      const current = config ? validateStaticQris(config.staticPayload) : null;
      if (!active) return;
      setSaved(current);

      const pending = await ImagePicker.getPendingResultAsync();
      if (!active || !pending) return;
      if ("code" in pending) {
        throw new Error(pending.message);
      }
      if (pending.canceled) return;

      const asset = pending.assets[0];
      if (!asset) {
        throw new Error("Gambar QRIS tidak tersedia.");
      }
      const parsed = await parseQrisImageAsset(asset);
      if (!active) return;
      if (current?.payload === parsed.payload) {
        setMessage("QRIS dari gambar tersebut sudah tersimpan.");
        return;
      }
      setCandidate(parsed);
      setReplacing(current !== null);
      setMessage("Gambar dipulihkan. Periksa merchant sebelum menyimpan.");
    })()
      .catch((reason: unknown) => {
        if (!active) return;
        setError(
          reason instanceof Error
            ? reason.message
            : "Konfigurasi QRIS tidak dapat dibaca.",
        );
      })
      .finally(() => {
        if (active) setLoading(false);
      });

    return () => {
      active = false;
    };
  }, [role]);

  if (role !== "superadmin") {
    return (
      <AppScreen>
        <PageHeader back title="QRIS Dinamis" />
        <StateView
          icon="shield-lock-outline"
          message="Hanya superadmin yang dapat mengubah identitas merchant QRIS."
          title="Akses dibatasi"
        />
      </AppScreen>
    );
  }

  if (loading) {
    return (
      <AppScreen>
        <PageHeader back title="QRIS Dinamis" />
        <StateView
          icon="qrcode-scan"
          message="Memeriksa QRIS yang tersimpan pada perangkat."
          title="Menyiapkan QRIS"
        />
      </AppScreen>
    );
  }

  const stageImage = async (source: PickerSource) => {
    if (pickerActiveRef.current) return;
    pickerActiveRef.current = true;
    setProcessingSource(source);
    setMessage(null);
    setError(null);

    try {
      let result: ImagePicker.ImagePickerResult;
      if (source === "camera") {
        const permission = await ImagePicker.requestCameraPermissionsAsync();
        if (!permission.granted) {
          throw new Error(
            permission.canAskAgain
              ? "Izin kamera diperlukan untuk memotret QRIS."
              : "Izin kamera ditolak. Aktifkan izin Kamera melalui Pengaturan perangkat.",
          );
        }
        result = await ImagePicker.launchCameraAsync({
          ...QRIS_IMAGE_OPTIONS,
          cameraType: ImagePicker.CameraType.back,
        });
      } else {
        result = await ImagePicker.launchImageLibraryAsync(QRIS_IMAGE_OPTIONS);
      }

      if (result.canceled) return;
      const asset = result.assets[0];
      if (!asset) {
        throw new Error("Gambar QRIS tidak tersedia.");
      }
      const parsed = await parseQrisImageAsset(asset);
      if (saved?.payload === parsed.payload) {
        setCandidate(null);
        setReplacing(false);
        setMessage("QRIS dari gambar tersebut sudah tersimpan.");
        return;
      }
      setCandidate(parsed);
      setMessage("QRIS berhasil dibaca. Periksa merchant sebelum menyimpan.");
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Gambar QRIS tidak dapat diproses.",
      );
    } finally {
      pickerActiveRef.current = false;
      setProcessingSource(null);
    }
  };

  const saveCandidate = async () => {
    if (!candidate || saveActiveRef.current) return;
    saveActiveRef.current = true;
    setSaving(true);
    setMessage(null);
    setError(null);
    try {
      await writeQrisConfig({ staticPayload: candidate.payload });
      setSaved(candidate);
      setCandidate(null);
      setReplacing(false);
      setMessage("QRIS statis tersimpan, terkunci, dan siap digunakan.");
    } catch (reason) {
      setError(
        reason instanceof Error
          ? reason.message
          : "Konfigurasi QRIS tidak dapat disimpan.",
      );
    } finally {
      saveActiveRef.current = false;
      setSaving(false);
    }
  };

  const cancelConfiguration = () => {
    setCandidate(null);
    setReplacing(false);
    setError(null);
    setMessage(
      saved ? "Penggantian dibatalkan. QRIS sebelumnya tetap aktif." : null,
    );
  };

  const beginReplace = () => {
    Alert.alert(
      "Ganti QRIS statis?",
      "Selesaikan pembayaran QRIS tertunda terlebih dahulu. Transaksi lama terikat pada QRIS sebelumnya dan kodenya tidak dapat dibuat ulang setelah penggantian. QRIS saat ini tetap aktif sampai QRIS baru berhasil disimpan.",
      [
        { text: "Batal", style: "cancel" },
        {
          text: "Lanjutkan",
          onPress: () => {
            setReplacing(true);
            setCandidate(null);
            setMessage(null);
            setError(null);
          },
        },
      ],
    );
  };

  const remove = () => {
    Alert.alert(
      "Hapus konfigurasi QRIS?",
      "Selesaikan pembayaran QRIS tertunda terlebih dahulu. Setelah dihapus, transaksi QRIS baru dan transaksi lama yang terikat QRIS ini tidak dapat menampilkan kode pembayaran.",
      [
        { text: "Batal", style: "cancel" },
        {
          text: "Hapus",
          style: "destructive",
          onPress: () => {
            if (saveActiveRef.current) return;
            saveActiveRef.current = true;
            setSaving(true);
            setError(null);
            setMessage(null);
            void clearQrisConfig()
              .then(() => {
                setSaved(null);
                setCandidate(null);
                setReplacing(false);
                setMessage("Konfigurasi QRIS dihapus dari perangkat.");
              })
              .catch((reason: unknown) =>
                setError(
                  reason instanceof Error
                    ? reason.message
                    : "Konfigurasi QRIS tidak dapat dihapus.",
                ),
              )
              .finally(() => {
                saveActiveRef.current = false;
                setSaving(false);
              });
          },
        },
      ],
    );
  };

  const processing = processingSource !== null;
  const showScanner = candidate === null && (saved === null || replacing);

  return (
    <AppScreen>
      <PageHeader
        back
        subtitle="Tersimpan aman pada perangkat ini"
        title="QRIS Dinamis"
      />
      <Card style={styles.info}>
        <Text style={styles.infoTitle}>Konfigurasi dari gambar</Text>
        <Text style={styles.infoText}>
          Ambil foto atau pilih gambar QRIS merchant. Pemindaian dilakukan
          langsung di perangkat; foto tidak diunggah atau disimpan. Aplikasi
          hanya menyimpan payload statis setelah struktur dan CRC valid.
        </Text>
      </Card>

      {candidate ? (
        <>
          <QrisSummaryCard qris={candidate} status="candidate" />
          <Text style={styles.reviewHint}>
            {saved
              ? "QRIS lama tetap aktif sampai konfigurasi baru disimpan."
              : "Payload hasil pemindaian terkunci dan tidak dapat diedit."}
          </Text>
          <Button
            disabled={processing}
            icon="content-save-check-outline"
            loading={saving}
            onPress={() => void saveCandidate()}
          >
            Simpan QRIS statis
          </Button>
          <Button
            disabled={saving || processing}
            icon="close"
            onPress={cancelConfiguration}
            variant="secondary"
          >
            Batalkan
          </Button>
        </>
      ) : saved && !replacing ? (
        <>
          <QrisSummaryCard qris={saved} status="saved" />
          <Button
            disabled={saving}
            icon="swap-horizontal"
            onPress={beginReplace}
            variant="secondary"
          >
            Ganti QRIS statis
          </Button>
          <Button
            disabled={saving}
            icon="delete-outline"
            loading={saving}
            onPress={remove}
            variant="danger"
          >
            Hapus konfigurasi QRIS
          </Button>
        </>
      ) : null}

      {showScanner ? (
        <Card style={styles.scannerCard}>
          <View style={styles.scannerIcon}>
            <Icon
              color={colors.primary}
              name="image-search-outline"
              size={30}
            />
          </View>
          <Text style={styles.scannerTitle}>
            {saved ? "Pindai QRIS pengganti" : "Tambahkan QRIS statis"}
          </Text>
          <Text style={styles.scannerText}>
            Potong gambar menjadi persegi dan pastikan kode QR memenuhi sebagian
            besar gambar agar mudah terbaca.
          </Text>
          <Button
            disabled={processing || saving}
            icon="camera-outline"
            loading={processingSource === "camera"}
            onPress={() => void stageImage("camera")}
          >
            Ambil foto QRIS
          </Button>
          <Button
            disabled={processing || saving}
            icon="image-outline"
            loading={processingSource === "gallery"}
            onPress={() => void stageImage("gallery")}
            variant="secondary"
          >
            Pilih gambar QRIS
          </Button>
          {saved ? (
            <Button
              disabled={processing || saving}
              icon="close"
              onPress={cancelConfiguration}
              variant="ghost"
            >
              Batal mengganti
            </Button>
          ) : null}
        </Card>
      ) : null}

      {error ? (
        <Text accessibilityRole="alert" style={styles.error}>
          {error}
        </Text>
      ) : null}
      {message ? (
        <Text accessibilityRole="alert" style={styles.message}>
          {message}
        </Text>
      ) : null}
      <Text style={styles.disclaimer}>
        CRC mendeteksi payload rusak, tetapi bukan bukti merchant telah
        diverifikasi oleh bank atau acquirer. Status pembayaran tetap harus
        dikonfirmasi melalui aplikasi/acquirer merchant.
      </Text>
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  info: { gap: spacing.xs, backgroundColor: colors.primarySoft },
  infoTitle: { ...textStyles.heading, color: colors.primary },
  infoText: { ...textStyles.body, color: colors.textMuted },
  savedCard: { gap: spacing.md, backgroundColor: colors.successSoft },
  candidateCard: { gap: spacing.md, backgroundColor: colors.primarySoft },
  lockHeader: {
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.sm,
  },
  lockIcon: {
    width: 44,
    height: 44,
    borderRadius: radius.md,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.card,
  },
  candidateIcon: {
    width: 44,
    height: 44,
    borderRadius: radius.md,
    alignItems: "center",
    justifyContent: "center",
    backgroundColor: colors.card,
  },
  lockCopy: { flex: 1, gap: 2 },
  lockHint: { ...textStyles.body, color: colors.textMuted, fontSize: 12 },
  qrFrame: {
    alignSelf: "center",
    backgroundColor: "#FFFFFF",
    padding: spacing.sm,
    borderRadius: radius.lg,
  },
  merchantCopy: { alignItems: "center", gap: spacing.xs },
  merchantName: {
    fontFamily: typography.heading,
    fontSize: 20,
    color: colors.text,
    textAlign: "center",
  },
  merchantCity: { ...textStyles.body, color: colors.textMuted },
  payloadReference: {
    minHeight: 36,
    flexDirection: "row",
    alignItems: "center",
    gap: spacing.xs,
    paddingHorizontal: spacing.sm,
    borderRadius: radius.sm,
    backgroundColor: colors.card,
  },
  payloadText: {
    flex: 1,
    color: colors.textMuted,
    fontFamily: typography.mono,
    fontSize: 11,
  },
  reviewHint: {
    ...textStyles.body,
    color: colors.textMuted,
    textAlign: "center",
  },
  scannerCard: { gap: spacing.md, alignItems: "stretch" },
  scannerIcon: {
    width: 56,
    height: 56,
    borderRadius: radius.lg,
    alignItems: "center",
    justifyContent: "center",
    alignSelf: "center",
    backgroundColor: colors.primarySoft,
  },
  scannerTitle: {
    ...textStyles.heading,
    color: colors.text,
    textAlign: "center",
  },
  scannerText: {
    ...textStyles.body,
    color: colors.textMuted,
    textAlign: "center",
  },
  error: { ...textStyles.body, color: colors.error, textAlign: "center" },
  message: { ...textStyles.body, color: colors.success, textAlign: "center" },
  disclaimer: {
    ...textStyles.body,
    color: colors.textMuted,
    textAlign: "center",
    fontSize: 12,
  },
});
