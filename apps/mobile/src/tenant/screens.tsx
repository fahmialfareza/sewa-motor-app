import { Redirect, useFocusEffect, useRouter } from "expo-router";
import { useCallback, useRef, useState } from "react";
import { Alert, StyleSheet, Text, View } from "react-native";
import { apiRequest } from "@/api/client";
import type { AuthContextsResponse, TenantInvitation } from "@/api/contracts";
import { useAuth } from "@/auth/AuthProvider";
import { AppScreen } from "@/components/layout/AppScreen";
import { PageHeader } from "@/components/layout/PageHeader";
import { Button } from "@/components/ui/Button";
import { Card } from "@/components/ui/Card";
import { Field } from "@/components/ui/Field";
import type {
  BusinessProfile,
  Role,
  TenantSummary,
  UserSummary,
} from "@/domain/types";
import { colors, spacing, textStyles } from "@/theme/tokens";
import { toUserFacingErrorMessage } from "@/utils/errors";
import { cacheTenantConfiguration } from "./configuration";

function useTask() {
  const [busy, setBusy] = useState(false);
  const [error, setError] = useState<string | null>(null);
  const running = useRef(false);
  const run = async (work: () => Promise<void>) => {
    if (running.current) return;
    running.current = true;
    setBusy(true);
    setError(null);
    try {
      await work();
    } catch (reason) {
      setError(
        toUserFacingErrorMessage(
          reason,
          "Permintaan belum berhasil. Coba lagi.",
        ),
      );
    } finally {
      running.current = false;
      setBusy(false);
    }
  };
  return { busy, error, run };
}
function ErrorMessage({ message }: { message: string | null }) {
  return message ? (
    <Text accessibilityRole="alert" style={styles.error}>
      {message}
    </Text>
  ) : null;
}
function RolePicker({
  role,
  onChange,
}: {
  role: Role;
  onChange: (role: Role) => void;
}) {
  return (
    <View style={styles.row}>
      {(["admin", "superadmin"] as const).map((value) => (
        <Button
          key={value}
          style={styles.flex}
          variant={role === value ? "primary" : "secondary"}
          onPress={() => onChange(value)}
        >
          {value === "admin" ? "Admin" : "Superadmin"}
        </Button>
      ))}
    </View>
  );
}

export function ContextsScreen() {
  const router = useRouter();
  const { session, scopeLocked, notice, switchContext, logout } = useAuth();
  const token = session?.token;
  const [contexts, setContexts] = useState<AuthContextsResponse | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [code, setCode] = useState("");
  const task = useTask();
  useFocusEffect(
    useCallback(() => {
      let active = true;
      if (token)
        void apiRequest<AuthContextsResponse>("/auth/contexts", { token })
          .then((value) => {
            if (active) {
              setContexts(value);
              setLoadError(null);
            }
          })
          .catch((reason) => {
            if (active)
              setLoadError(
                toUserFacingErrorMessage(
                  reason,
                  "Permintaan belum berhasil. Coba lagi.",
                ),
              );
          });
      return () => {
        active = false;
      };
    }, [token]),
  );
  if (!session) return <Redirect href="/(auth)/login" />;
  if (session.user.mustChangePassword)
    return <Redirect href="/(auth)/change-password" />;
  return (
    <AppScreen>
      <PageHeader
        title="Pilih bisnis"
        subtitle={`Masuk sebagai ${session.user.fullName}`}
      />
      {scopeLocked ? (
        <Card>
          <Text style={textStyles.heading}>
            Data belum tersinkron diamankan
          </Text>
          <Text style={styles.body}>
            {notice ??
              "Akses bisnis ini dihentikan. Hubungi pengelola untuk memulihkan akses. Anda dapat memilih bisnis lain."}
          </Text>
        </Card>
      ) : null}
      <ErrorMessage message={loadError ?? task.error} />
      {!contexts ? (
        <Button
          loading={task.busy}
          onPress={() =>
            void task.run(async () => {
              setContexts(
                await apiRequest<AuthContextsResponse>("/auth/contexts", {
                  token: session.token,
                }),
              );
              setLoadError(null);
            })
          }
        >
          Muat daftar bisnis
        </Button>
      ) : null}
      {contexts?.tenants.map(({ tenant, role }) => (
        <Card key={tenant.id} style={styles.card}>
          <Text style={textStyles.heading}>{tenant.name}</Text>
          <Text style={styles.body}>
            {role === "superadmin" ? "Superadmin" : "Admin"} •{" "}
            {tenant.status === "active"
              ? "Aktif"
              : tenant.status === "suspended"
                ? "Ditangguhkan"
                : "Menunggu pemilik"}
          </Text>
          <Button
            disabled={tenant.status !== "active" || task.busy}
            onPress={() =>
              void task.run(async () => {
                await switchContext("tenant", tenant.id);
                router.replace("/");
              })
            }
          >
            Buka bisnis
          </Button>
        </Card>
      ))}
      {contexts && contexts.tenants.length === 0 ? (
        <Text style={styles.body}>
          Anda belum bergabung dengan bisnis. Masukkan kode undangan dari
          pengelola.
        </Text>
      ) : null}
      {contexts?.platformAdmin ? (
        <Button
          variant="secondary"
          disabled={task.busy}
          onPress={() =>
            void task.run(async () => {
              await switchContext("platform");
              router.replace("/platform");
            })
          }
        >
          Kelola platform
        </Button>
      ) : null}
      <Card style={styles.card}>
        <Text style={textStyles.heading}>Gabung dengan undangan</Text>
        <Field
          label="Kode undangan"
          autoCapitalize="none"
          autoCorrect={false}
          value={code}
          onChangeText={setCode}
        />
        <Button
          loading={task.busy}
          disabled={!code.trim()}
          onPress={() =>
            void task.run(async () => {
              await apiRequest("/auth/invitations/accept", {
                method: "POST",
                token: session.token,
                body: { code: code.trim() },
              });
              setCode("");
              setContexts(
                await apiRequest<AuthContextsResponse>("/auth/contexts", {
                  token: session.token,
                }),
              );
            })
          }
        >
          Terima undangan
        </Button>
      </Card>
      <Button
        variant="secondary"
        onPress={() => router.push("/account-profile")}
      >
        Profil dan kata sandi saya
      </Button>
      {!scopeLocked &&
      (!session.contextKind || session.contextKind === "tenant") ? (
        <Button variant="ghost" onPress={() => router.replace("/")}>
          Kembali ke bisnis aktif
        </Button>
      ) : null}
      <Button
        variant="ghost"
        loading={task.busy}
        onPress={() =>
          void task.run(async () => {
            await logout();
            router.replace("/");
          })
        }
      >
        Keluar
      </Button>
    </AppScreen>
  );
}

export function RegisterInvitationScreen() {
  const router = useRouter();
  const { registerInvitation, session } = useAuth();
  const [code, setCode] = useState("");
  const [username, setUsername] = useState("");
  const [fullName, setFullName] = useState("");
  const [password, setPassword] = useState("");
  const task = useTask();
  if (session) return <Redirect href="/contexts" />;
  return (
    <AppScreen authenticated={false}>
      <PageHeader back title="Daftar dengan undangan" />
      <Text style={styles.body}>
        Gunakan akun yang sama untuk semua bisnis. Jika sudah memiliki akun,
        masuk lalu terima undangan di Pilih bisnis.
      </Text>
      <Card style={styles.card}>
        <Field
          label="Kode undangan"
          autoCapitalize="none"
          value={code}
          onChangeText={setCode}
        />
        <Field
          label="Nama lengkap"
          value={fullName}
          onChangeText={setFullName}
        />
        <Field
          label="Nama pengguna"
          autoCapitalize="none"
          autoCorrect={false}
          value={username}
          onChangeText={setUsername}
        />
        <Field
          label="Kata sandi"
          secureTextEntry
          value={password}
          onChangeText={setPassword}
        />
        <ErrorMessage message={task.error} />
        <Button
          loading={task.busy}
          disabled={
            !code.trim() || !username.trim() || !fullName.trim() || !password
          }
          onPress={() =>
            void task.run(async () => {
              await registerInvitation(code, username, fullName, password);
              router.replace("/");
            })
          }
        >
          Daftar dan gabung
        </Button>
      </Card>
    </AppScreen>
  );
}

export function InvitationsScreen() {
  const { session } = useAuth();
  const token = session?.token;
  const [invitations, setInvitations] = useState<TenantInvitation[]>([]);
  const [role, setRole] = useState<Role>("admin");
  const [issued, setIssued] = useState<TenantInvitation | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const task = useTask();
  useFocusEffect(
    useCallback(() => {
      let active = true;
      if (token)
        void apiRequest<TenantInvitation[]>("/tenant/invitations", { token })
          .then((value) => {
            if (active) setInvitations(value);
          })
          .catch((reason) => {
            if (active)
              setLoadError(
                toUserFacingErrorMessage(
                  reason,
                  "Permintaan belum berhasil. Coba lagi.",
                ),
              );
          });
      return () => {
        active = false;
      };
    }, [token]),
  );
  if (
    !session ||
    session.user.role !== "superadmin" ||
    session.dataMode !== "production"
  )
    return <Redirect href="/" />;
  const reload = async () => {
    setInvitations(
      await apiRequest<TenantInvitation[]>("/tenant/invitations", {
        token: session.token,
      }),
    );
    setLoadError(null);
  };
  return (
    <AppScreen>
      <PageHeader
        back
        title="Undangan staf"
        subtitle="Kode sekali pakai berlaku selama 7 hari"
      />
      <Card style={styles.card}>
        <RolePicker role={role} onChange={setRole} />
        <Button
          loading={task.busy}
          onPress={() =>
            void task.run(async () => {
              setIssued(
                await apiRequest<TenantInvitation>("/tenant/invitations", {
                  method: "POST",
                  token: session.token,
                  body: { role },
                }),
              );
              await reload();
            })
          }
        >
          Buat kode undangan
        </Button>
      </Card>
      {issued?.code ? (
        <Card style={styles.card}>
          <Text style={textStyles.heading}>Bagikan kode ini kepada staf</Text>
          <Text selectable style={textStyles.technical}>
            {issued.code}
          </Text>
          <Text style={styles.body}>
            Kode hanya ditampilkan saat dibuat. Kedaluwarsa{" "}
            {new Date(issued.expiresAt).toLocaleString("id-ID")}.
          </Text>
        </Card>
      ) : null}
      <ErrorMessage message={task.error ?? loadError} />
      {invitations.map((item) => (
        <Card key={item.id} style={styles.card}>
          <Text style={textStyles.heading}>{item.role}</Text>
          <Text style={styles.body}>
            {item.acceptedAt
              ? "Sudah diterima"
              : item.revokedAt
                ? "Dicabut"
                : `Berlaku sampai ${new Date(item.expiresAt).toLocaleString("id-ID")}`}
          </Text>
          {!item.acceptedAt && !item.revokedAt ? (
            <Button
              disabled={task.busy}
              variant="danger"
              onPress={() =>
                void task.run(async () => {
                  await apiRequest(`/tenant/invitations/${item.id}`, {
                    method: "DELETE",
                    token: session.token,
                  });
                  if (issued?.id === item.id) setIssued(null);
                  await reload();
                })
              }
            >
              Cabut undangan
            </Button>
          ) : null}
        </Card>
      ))}
    </AppScreen>
  );
}

export function MembershipEditor({ id }: { id: string }) {
  const router = useRouter();
  const { session } = useAuth();
  const token = session?.token;
  const [member, setMember] = useState<UserSummary | null>(null);
  const [role, setRole] = useState<Role>("admin");
  const [active, setActive] = useState(true);
  const [loadError, setLoadError] = useState<string | null>(null);
  const task = useTask();
  useFocusEffect(
    useCallback(() => {
      let mounted = true;
      if (token)
        void apiRequest<UserSummary[]>("/tenant/members", { token })
          .then((items) => {
            if (!mounted) return;
            const value = items.find((item) => item.id === id);
            if (!value) throw new Error("Anggota tidak ditemukan.");
            setMember(value);
            setRole(value.role);
            setActive(value.active);
          })
          .catch((reason) => {
            if (mounted)
              setLoadError(
                toUserFacingErrorMessage(
                  reason,
                  "Permintaan belum berhasil. Coba lagi.",
                ),
              );
          });
      return () => {
        mounted = false;
      };
    }, [token, id]),
  );
  if (
    !session ||
    session.dataMode !== "production" ||
    session.user.role !== "superadmin"
  )
    return <Redirect href="/" />;
  return (
    <AppScreen>
      <PageHeader
        back
        title="Akses anggota"
        subtitle="Perubahan hanya berlaku untuk bisnis ini"
      />
      {loadError ? (
        <Card style={styles.card}>
          <Text style={textStyles.heading}>Pengguna belum dapat dimuat</Text>
          <ErrorMessage message={loadError} />
          <Button
            loading={task.busy}
            onPress={() =>
              void task.run(async () => {
                const items = await apiRequest<UserSummary[]>(
                  "/tenant/members",
                  { token: session.token },
                );
                const value = items.find((item) => item.id === id);
                if (!value) throw new Error("Anggota tidak ditemukan.");
                setMember(value);
                setRole(value.role);
                setActive(value.active);
                setLoadError(null);
              })
            }
          >
            Coba lagi
          </Button>
        </Card>
      ) : null}
      <ErrorMessage message={task.error} />
      {member ? (
        <Card style={styles.card}>
          <Text style={textStyles.heading}>{member.fullName}</Text>
          <Text style={styles.body}>@{member.username}</Text>
          <RolePicker role={role} onChange={setRole} />
          <Button
            variant="secondary"
            onPress={() => setActive((value) => !value)}
          >
            {active
              ? "Akses aktif — ketuk untuk nonaktifkan"
              : "Akses nonaktif — ketuk untuk aktifkan"}
          </Button>
          <Button
            loading={task.busy}
            onPress={() =>
              void task.run(async () => {
                await apiRequest(`/tenant/members/${id}`, {
                  method: "PATCH",
                  token: session.token,
                  body: { role, active },
                });
                router.back();
              })
            }
          >
            Simpan akses
          </Button>
        </Card>
      ) : null}
    </AppScreen>
  );
}

export function BusinessProfileScreen() {
  const { session } = useAuth();
  const token = session?.token;
  const [profile, setProfile] = useState<BusinessProfile | null>(null);
  const [saved, setSaved] = useState(false);
  const [loadError, setLoadError] = useState<string | null>(null);
  const task = useTask();
  useFocusEffect(
    useCallback(() => {
      let active = true;
      if (token)
        void apiRequest<BusinessProfile>("/tenant/profile", { token })
          .then((value) => {
            if (active) setProfile(value);
          })
          .catch((reason) => {
            if (active)
              setLoadError(
                toUserFacingErrorMessage(
                  reason,
                  "Permintaan belum berhasil. Coba lagi.",
                ),
              );
          });
      return () => {
        active = false;
      };
    }, [token]),
  );
  if (
    !session ||
    session.dataMode !== "production" ||
    session.user.role !== "superadmin"
  )
    return <Redirect href="/" />;
  return (
    <AppScreen>
      <PageHeader
        back
        title="Identitas bisnis"
        subtitle="Digunakan pada transaksi baru, struk, dan laporan"
      />
      <ErrorMessage message={task.error ?? loadError} />
      {profile ? (
        <Card style={styles.card}>
          <Field
            label="Nama bisnis"
            value={profile.businessName}
            onChangeText={(businessName) => {
              setSaved(false);
              setProfile({ ...profile, businessName });
            }}
          />
          <Field
            label="Alamat (opsional)"
            value={profile.address ?? ""}
            onChangeText={(address) => {
              setSaved(false);
              setProfile({ ...profile, address });
            }}
          />
          <Field
            label="Telepon (opsional)"
            keyboardType="phone-pad"
            value={profile.phone ?? ""}
            onChangeText={(phone) => {
              setSaved(false);
              setProfile({ ...profile, phone });
            }}
          />
          <Button
            loading={task.busy}
            disabled={!profile.businessName.trim()}
            onPress={() =>
              void task.run(async () => {
                const value = await apiRequest<BusinessProfile>(
                  "/tenant/profile",
                  {
                    method: "PATCH",
                    token: session.token,
                    body: {
                      businessName: profile.businessName.trim(),
                      address: profile.address?.trim() || null,
                      phone: profile.phone?.trim() || null,
                      expectedRevision: profile.revision,
                    },
                  },
                );
                await cacheTenantConfiguration("profile", value, session);
                setProfile(value);
                setSaved(true);
              })
            }
          >
            Simpan identitas
          </Button>
          {saved ? (
            <Text style={styles.body}>
              Identitas bisnis tersimpan. Struk transaksi lama tetap menggunakan
              identitas saat dibuat.
            </Text>
          ) : null}
        </Card>
      ) : null}
    </AppScreen>
  );
}

interface PlatformAudit {
  id: string;
  eventType: string;
  createdAt: string;
}
export function PlatformScreen() {
  const { session, switchContext } = useAuth();
  const token = session?.token;
  const router = useRouter();
  const [tenants, setTenants] = useState<TenantSummary[]>([]);
  const [audit, setAudit] = useState<PlatformAudit[]>([]);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [invitation, setInvitation] = useState<TenantInvitation | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const task = useTask();
  useFocusEffect(
    useCallback(() => {
      let active = true;
      if (token)
        void apiRequest<TenantSummary[]>("/platform/tenants", { token })
          .then((value) => {
            if (active) setTenants(value);
          })
          .catch((reason) => {
            if (active)
              setLoadError(
                toUserFacingErrorMessage(
                  reason,
                  "Permintaan belum berhasil. Coba lagi.",
                ),
              );
          });
      return () => {
        active = false;
      };
    }, [token]),
  );
  if (!session) return <Redirect href="/(auth)/login" />;
  if (session.contextKind !== "platform") return <Redirect href="/contexts" />;
  const reload = async () => {
    setTenants(
      await apiRequest<TenantSummary[]>("/platform/tenants", {
        token: session.token,
      }),
    );
    setLoadError(null);
  };
  return (
    <AppScreen>
      <PageHeader
        title="Pengelolaan platform"
        subtitle="Penyediaan dan status bisnis"
      />
      <Button
        variant="secondary"
        onPress={() =>
          void task.run(async () => {
            await switchContext("account");
            router.replace("/contexts");
          })
        }
      >
        Pilih konteks lain
      </Button>
      <Card style={styles.card}>
        <Text style={textStyles.heading}>Buat bisnis</Text>
        <Field label="Nama bisnis" value={name} onChangeText={setName} />
        <Field
          label="Kode bisnis"
          autoCapitalize="none"
          autoCorrect={false}
          hint="Huruf kecil, angka, dan tanda hubung"
          value={slug}
          onChangeText={setSlug}
        />
        <Button
          loading={task.busy}
          disabled={!name.trim() || !slug.trim()}
          onPress={() =>
            void task.run(async () => {
              const value = await apiRequest<{
                tenant: TenantSummary;
                invitation: TenantInvitation;
              }>("/platform/tenants", {
                method: "POST",
                token: session.token,
                body: { name: name.trim(), slug: slug.trim() },
              });
              setInvitation(value.invitation);
              setName("");
              setSlug("");
              await reload();
            })
          }
        >
          Buat dan undang pemilik
        </Button>
      </Card>
      {invitation?.code ? (
        <Card style={styles.card}>
          <Text style={textStyles.heading}>Kode untuk pemilik bisnis</Text>
          <Text selectable style={textStyles.technical}>
            {invitation.code}
          </Text>
          <Text style={styles.body}>
            Bagikan secara pribadi kepada pemilik. Bisnis aktif setelah kode
            diterima. Kode berlaku 7 hari dan hanya ditampilkan sekarang.
          </Text>
        </Card>
      ) : null}
      <ErrorMessage message={task.error ?? loadError} />
      {tenants.map((tenant) => (
        <Card key={tenant.id} style={styles.card}>
          <Text style={textStyles.heading}>{tenant.name}</Text>
          <Text style={styles.body}>
            {tenant.slug} •{" "}
            {tenant.status === "active"
              ? "Aktif"
              : tenant.status === "suspended"
                ? "Ditangguhkan"
                : "Menunggu pemilik"}
          </Text>
          {tenant.status === "pending_setup" ? (
            <Button
              disabled={task.busy}
              variant="secondary"
              onPress={() =>
                void task.run(async () => {
                  setInvitation(
                    await apiRequest<TenantInvitation>(
                      `/platform/tenants/${tenant.id}/invitation`,
                      { method: "POST", token: session.token },
                    ),
                  );
                })
              }
            >
              Ganti undangan pemilik
            </Button>
          ) : (
            <Button
              disabled={task.busy}
              variant={tenant.status === "active" ? "danger" : "secondary"}
              onPress={() =>
                Alert.alert(
                  tenant.status === "active"
                    ? "Tangguhkan bisnis?"
                    : "Aktifkan bisnis?",
                  tenant.status === "active"
                    ? "Seluruh akses bisnis akan dihentikan. Perangkat offline mengamankan antrean saat terhubung kembali."
                    : "Staf perlu memilih bisnis kembali untuk mendapatkan sesi baru.",
                  [
                    { text: "Batal", style: "cancel" },
                    {
                      text: "Konfirmasi",
                      onPress: () =>
                        void task.run(async () => {
                          await apiRequest(
                            `/platform/tenants/${tenant.id}/status`,
                            {
                              method: "POST",
                              token: session.token,
                              body: {
                                status:
                                  tenant.status === "active"
                                    ? "suspended"
                                    : "active",
                              },
                            },
                          );
                          await reload();
                        }),
                    },
                  ],
                )
              }
            >
              {tenant.status === "active" ? "Tangguhkan" : "Aktifkan kembali"}
            </Button>
          )}
        </Card>
      ))}
      <Button
        variant="secondary"
        loading={task.busy}
        onPress={() =>
          void task.run(async () => {
            setAudit(
              await apiRequest<PlatformAudit[]>("/platform/audit", {
                token: session.token,
              }),
            );
          })
        }
      >
        Muat riwayat platform
      </Button>
      {audit.map((event) => (
        <Card key={event.id}>
          <Text style={styles.body}>{event.eventType}</Text>
          <Text style={styles.body}>
            {new Date(event.createdAt).toLocaleString("id-ID")}
          </Text>
        </Card>
      ))}
    </AppScreen>
  );
}
const styles = StyleSheet.create({
  card: { gap: spacing.md },
  body: { ...textStyles.body, color: colors.textMuted },
  error: { ...textStyles.body, color: colors.error },
  row: { flexDirection: "row", gap: spacing.sm },
  flex: { flex: 1 },
});
