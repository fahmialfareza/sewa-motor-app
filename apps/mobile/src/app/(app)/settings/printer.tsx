import { useEffect, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { createPrinter } from "@/printer/service";
import type { PrinterDevice } from "@/printer/types";
import {
  readPrinterConfig,
  writePrinterConfig,
  type PrinterConfig,
} from "@/security/secure-store";
import {
  colors,
  radius,
  spacing,
  textStyles,
  typography,
} from "@/theme/tokens";

const adapterLabels: Record<PrinterConfig["adapter"], string> = {
  simulator: "Simulator",
  bluetooth: "Bluetooth ESC/POS",
  integrated: "Printer MPOS",
};

export default function PrinterSettingsScreen() {
  const [config, setConfig] = useState<PrinterConfig | null>(null);
  const [devices, setDevices] = useState<PrinterDevice[]>([]);
  const [message, setMessage] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);

  useEffect(() => {
    void readPrinterConfig().then(setConfig);
  }, []);
  if (!config) return <AppScreen />;

  const discover = async () => {
    setBusy(true);
    setMessage(null);
    try {
      const found = await createPrinter(config).discover();
      setDevices(found);
      setMessage(`${found.length} printer ditemukan.`);
    } catch (reason) {
      setMessage(reason instanceof Error ? reason.message : "Pencarian gagal.");
    } finally {
      setBusy(false);
    }
  };

  const save = async () => {
    await writePrinterConfig(config);
    setMessage("Konfigurasi printer tersimpan.");
  };

  return (
    <AppScreen>
      <PageHeader
        back
        subtitle="Development build diperlukan"
        title="Pengaturan Printer"
      />
      <Text style={textStyles.label}>JENIS PRINTER</Text>
      <View style={styles.options}>
        {(Object.keys(adapterLabels) as PrinterConfig["adapter"][]).map(
          (adapter) => (
            <Pressable
              key={adapter}
              onPress={() =>
                setConfig((current) =>
                  current
                    ? {
                        ...current,
                        adapter,
                        address: null,
                        displayName: adapterLabels[adapter],
                      }
                    : current,
                )
              }
              style={[
                styles.option,
                config.adapter === adapter && styles.selected,
              ]}
            >
              <Text
                style={[
                  styles.optionText,
                  config.adapter === adapter && { color: colors.primary },
                ]}
              >
                {adapterLabels[adapter]}
              </Text>
            </Pressable>
          ),
        )}
      </View>
      <Text style={textStyles.label}>LEBAR KERTAS</Text>
      <View style={styles.options}>
        {([32, 48] as const).map((columns) => (
          <Pressable
            key={columns}
            onPress={() => setConfig({ ...config, paperColumns: columns })}
            style={[
              styles.option,
              config.paperColumns === columns && styles.selected,
            ]}
          >
            <Text style={styles.optionText}>
              {columns === 32 ? "58 mm • 32 kolom" : "80 mm • 48 kolom"}
            </Text>
          </Pressable>
        ))}
      </View>
      {config.adapter !== "simulator" ? (
        <Button
          icon="bluetooth-connect"
          loading={busy}
          onPress={() => void discover()}
          variant="secondary"
        >
          Temukan printer
        </Button>
      ) : null}
      {devices.map((device) => (
        <Pressable
          key={device.id}
          onPress={() =>
            setConfig({
              ...config,
              address: device.id,
              displayName: device.name,
            })
          }
        >
          <Card
            style={
              config.address === device.id ? styles.deviceSelected : undefined
            }
          >
            <Text style={styles.deviceName}>{device.name}</Text>
            <Text style={styles.deviceId}>{device.id}</Text>
          </Card>
        </Pressable>
      ))}
      <Card style={styles.current}>
        <Text style={textStyles.label}>KONFIGURASI AKTIF</Text>
        <Text style={styles.deviceName}>{config.displayName}</Text>
        <Text style={styles.deviceId}>
          {config.address ?? "Tidak memerlukan alamat perangkat"}
        </Text>
      </Card>
      {message ? <Text style={styles.message}>{message}</Text> : null}
      <Button icon="content-save-outline" onPress={() => void save()}>
        Simpan konfigurasi
      </Button>
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  options: { flexDirection: "row", flexWrap: "wrap", gap: spacing.sm },
  option: {
    minHeight: 48,
    paddingHorizontal: spacing.md,
    justifyContent: "center",
    borderRadius: radius.md,
    borderWidth: 1,
    borderColor: colors.outline,
    backgroundColor: colors.card,
  },
  selected: { borderColor: colors.primary, borderWidth: 2 },
  optionText: { ...textStyles.body, fontFamily: typography.bodyMedium },
  deviceSelected: { borderColor: colors.primary, borderWidth: 2 },
  deviceName: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    color: colors.text,
  },
  deviceId: { ...textStyles.technical, marginTop: spacing.xs },
  current: { gap: spacing.xs, backgroundColor: colors.primarySoft },
  message: { ...textStyles.body, color: colors.secondary },
});
