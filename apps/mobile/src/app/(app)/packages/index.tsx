import { useFocusEffect, useRouter } from "expo-router";
import { useCallback, useState } from "react";
import { Pressable, StyleSheet, Text, View } from "react-native";

import { apiRequest } from "@/api/client";
import type { PackageListResponse } from "@/api/contracts";
import { mapApiPackage } from "@/api/mappers";
import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { listPackages, upsertPackage } from "@/db/repositories";
import type { RentalPackage } from "@/domain/types";
import { colors, spacing, textStyles, typography } from "@/theme/tokens";
import { formatRupiah } from "@/utils/format";

export default function PackagesScreen() {
  const router = useRouter();
  const { session } = useAuth();
  const [packages, setPackages] = useState<RentalPackage[]>([]);

  const load = useCallback(async () => {
    setPackages(await listPackages(true));
    if (session && !session.token.startsWith("dev-only-")) {
      const remote = await apiRequest<PackageListResponse>(
        "/packages?limit=100",
        {
          token: session.token,
        },
      );
      const mapped = remote.map(mapApiPackage);
      await Promise.all(mapped.map(upsertPackage));
      setPackages(await listPackages(true));
    }
  }, [session]);

  useFocusEffect(
    useCallback(() => {
      void load();
    }, [load]),
  );

  return (
    <AppScreen>
      <PageHeader
        back
        subtitle="Superadmin • online untuk perubahan"
        title="Paket & Harga"
      />
      <Button
        icon="tag-plus-outline"
        onPress={() => router.push("/packages/new")}
      >
        Tambah paket
      </Button>
      {packages.map((item) => (
        <Pressable
          key={item.id}
          onPress={() =>
            router.push({
              pathname: "/packages/[id]/edit",
              params: { id: item.id },
            })
          }
        >
          <Card style={styles.package}>
            <View
              style={[
                styles.accent,
                {
                  backgroundColor:
                    item.accent === "sunrise"
                      ? colors.sunrise
                      : item.accent === "standard"
                        ? colors.standard
                        : colors.primary,
                },
              ]}
            />
            <View style={styles.copy}>
              <Text style={styles.name}>{item.name}</Text>
              <Text style={styles.description}>{item.description}</Text>
              <Text style={styles.revision}>REVISI {item.revision}</Text>
            </View>
            <View style={styles.right}>
              <Text style={styles.price}>{formatRupiah(item.unitPrice)}</Text>
              <Text style={item.active ? styles.active : styles.inactive}>
                {item.active ? "AKTIF" : "NONAKTIF"}
              </Text>
            </View>
          </Card>
        </Pressable>
      ))}
    </AppScreen>
  );
}

const styles = StyleSheet.create({
  package: {
    paddingLeft: spacing.lg,
    flexDirection: "row",
    alignItems: "center",
    overflow: "hidden",
    gap: spacing.sm,
  },
  accent: { position: "absolute", left: 0, top: 0, bottom: 0, width: 5 },
  copy: { flex: 1 },
  name: {
    fontFamily: typography.headingSemibold,
    fontSize: 16,
    color: colors.text,
  },
  description: { ...textStyles.body, color: colors.textMuted, fontSize: 12 },
  revision: { ...textStyles.label, fontSize: 9, marginTop: spacing.xs },
  right: { alignItems: "flex-end" },
  price: {
    fontFamily: typography.heading,
    color: colors.primary,
    fontSize: 16,
  },
  active: { ...textStyles.label, color: colors.success, fontSize: 9 },
  inactive: { ...textStyles.label, color: colors.textMuted, fontSize: 9 },
});
