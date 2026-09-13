import { Redirect, useFocusEffect, useRouter } from "expo-router";
import { useCallback, useRef, useState } from "react";
import { Alert, StyleSheet, Text, View } from "react-native";
import { apiRequest } from "@/api/client";
import type { AuthContextsResponse } from "@/api/contracts";
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
  disabled = false,
}: {
  role: Role;
  onChange: (role: Role) => void;
  disabled?: boolean;
}) {
  return (
    <View style={styles.row}>
      {(["admin", "superadmin"] as const).map((value) => (
        <Button
          key={value}
          disabled={disabled}
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

/** Only focused, authorized screens fetch; late responses cannot update a new context. */
function useRemote<T>(path: string, token?: string) {
  const [value, setValue] = useState<T | null>(null);
  const [error, setError] = useState<string | null>(null);
  const sequence = useRef(0);
  const reload = useCallback(async () => {
    const request = ++sequence.current;
    if (!token) return;
    try {
      const next = await apiRequest<T>(path, { token });
      if (request === sequence.current) {
        setValue(next);
        setError(null);
      }
    } catch (reason) {
      if (request === sequence.current)
        setError(
          toUserFacingErrorMessage(
            reason,
            "Data belum dapat dimuat. Coba lagi.",
          ),
        );
    }
  }, [path, token]);
  useFocusEffect(
    useCallback(() => {
      setValue(null);
      void reload();
      return () => {
        sequence.current += 1;
      };
    }, [reload]),
  );
  return { value, error, reload, setValue };
}

export function ContextsScreen() {
  const router = useRouter();
  const { session, scopeLocked, notice, switchContext, logout } = useAuth();
  const contexts = useRemote<AuthContextsResponse>(
    "/auth/contexts",
    session?.token,
  );
  const task = useTask();
  if (!session) return <Redirect href="/(auth)/login" />;
  if (session.user.mustChangePassword)
    return <Redirect href="/(auth)/change-password" />;
  return (
    <AppScreen>
      <PageHeader
        title="Pilih bisnis"
        subtitle={`Masuk sebagai ${session.user.fullName}`}
      />
      <Text style={styles.body}>
        Setiap akun aktif dapat mengakses semua bisnis aktif. Peran Anda berlaku
        di seluruh bisnis.
      </Text>
      {scopeLocked ? (
        <Card style={styles.card}>
          <Text style={textStyles.heading}>
            Data belum tersinkron diamankan
          </Text>
          <Text style={styles.body}>
            {notice ??
              "Akses bisnis ini dihentikan. Anda dapat memilih bisnis lain tanpa menghapus antrean lama."}
          </Text>
        </Card>
      ) : null}
      <ErrorMessage message={contexts.error ?? task.error} />
      <Button
        variant="secondary"
        disabled={task.busy}
        onPress={() => void contexts.reload()}
      >
        Muat ulang daftar bisnis
      </Button>
      {contexts.value?.tenants.map(({ tenant, role }) => (
        <Card key={tenant.id} style={styles.card}>
          <Text style={textStyles.heading}>
            {tenant.name}
            {session.tenantId === tenant.id ? " • Dipilih" : ""}
          </Text>
          <Text style={styles.body}>
            {role === "superadmin" ? "Superadmin" : "Admin"} •{" "}
            {tenantStatusLabel(tenant.status)}
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
      {contexts.value?.tenants.length === 0 ? (
        <Text style={styles.body}>
          Belum ada bisnis aktif. Hubungi Superadmin untuk membuat atau
          mengaktifkan bisnis.
        </Text>
      ) : null}
      {contexts.value?.canManageOrganization ? (
        <>
          <Button
            disabled={task.busy}
            onPress={() =>
              void task.run(async () => {
                if (session.contextKind !== "account")
                  await switchContext("account");
                router.replace("/management/tenants");
              })
            }
          >
            Kelola tenant
          </Button>
          <Button
            variant="secondary"
            disabled={task.busy}
            onPress={() =>
              void task.run(async () => {
                if (session.contextKind !== "account")
                  await switchContext("account");
                router.replace("/management/users");
              })
            }
          >
            Kelola pengguna
          </Button>
        </>
      ) : null}
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

function tenantStatusLabel(status: TenantSummary["status"]) {
  return status === "active"
    ? "Aktif"
    : status === "suspended"
      ? "Ditangguhkan"
      : "Menunggu aktivasi";
}

/** Visible entry point, not an automatic context exchange when a tab is focused. */
export function ManagementEntryScreen() {
  const router = useRouter();
  const { session, switchContext } = useAuth();
  const task = useTask();
  if (!session) return <Redirect href="/(auth)/login" />;
  if (session.user.role !== "superadmin") return <Redirect href="/" />;
  return (
    <AppScreen>
      <PageHeader
        title="Pengguna"
        subtitle="Akun dan peran bersama untuk seluruh bisnis"
      />
      <Card style={styles.card}>
        <Text style={styles.body}>
          Pengelolaan akun berlaku untuk seluruh Pengelola Wisata Telomoyo,
          bukan hanya bisnis yang dipilih. Perubahan lokal akan disinkronkan
          sebelum membuka pengelolaan.
        </Text>
        <Button
          loading={task.busy}
          onPress={() =>
            void task.run(async () => {
              if (session.contextKind !== "account")
                await switchContext("account");
              router.replace("/management/users");
            })
          }
        >
          Buka pengelolaan pengguna
        </Button>
        <ErrorMessage message={task.error} />
      </Card>
    </AppScreen>
  );
}

export function ManagedTenantsScreen() {
  const router = useRouter();
  const { session } = useAuth();
  const allowed =
    session?.contextKind === "account" && session.user.role === "superadmin";
  const token = allowed ? session.token : undefined;
  const tenants = useRemote<TenantSummary[]>("/management/tenants", token);
  const contexts = useRemote<AuthContextsResponse>("/auth/contexts", token);
  const [name, setName] = useState("");
  const [slug, setSlug] = useState("");
  const [editing, setEditing] = useState<TenantSummary | null>(null);
  const [editName, setEditName] = useState("");
  const task = useTask();
  if (!allowed) return <Redirect href="/contexts" />;
  return (
    <AppScreen>
      <PageHeader
        title="Kelola tenant"
        subtitle="Bisnis dalam Pengelola Wisata Telomoyo"
      />
      <Button variant="secondary" onPress={() => router.replace("/contexts")}>
        Ganti bisnis
      </Button>
      <Button variant="ghost" onPress={() => router.push("/management/users")}>
        Kelola pengguna
      </Button>
      <Card style={styles.card}>
        <Text style={textStyles.heading}>Buat bisnis</Text>
        {contexts.value?.tenantProvisioningEnabled ? (
          <>
            <Field
              label="Nama bisnis"
              value={name}
              onChangeText={setName}
              editable={!task.busy}
            />
            <Field
              label="Kode bisnis"
              value={slug}
              onChangeText={setSlug}
              editable={!task.busy}
              autoCapitalize="none"
              autoCorrect={false}
              hint="Huruf kecil, angka, dan tanda hubung. Tidak dapat diubah."
            />
            <Button
              loading={task.busy}
              disabled={!name.trim() || !slug.trim()}
              onPress={() =>
                void task.run(async () => {
                  await apiRequest<TenantSummary>("/management/tenants", {
                    method: "POST",
                    token: session.token,
                    body: { name: name.trim(), slug: slug.trim() },
                  });
                  setName("");
                  setSlug("");
                  await tenants.reload();
                })
              }
            >
              Buat bisnis aktif
            </Button>
            <Text style={styles.body}>
              Bisnis langsung dapat diakses semua staf dengan katalog kosong.
            </Text>
          </>
        ) : (
          <Text style={styles.body}>
            {contexts.value
              ? "Pembuatan bisnis belum diaktifkan pada server. Bisnis yang sudah ada tetap dapat dikelola."
              : "Memuat ketersediaan pembuatan bisnis…"}
          </Text>
        )}
      </Card>
      <ErrorMessage message={task.error ?? tenants.error ?? contexts.error} />
      <Button
        variant="secondary"
        onPress={() => void Promise.all([tenants.reload(), contexts.reload()])}
      >
        Muat ulang
      </Button>
      {tenants.value?.map((tenant) => (
        <Card key={tenant.id} style={styles.card}>
          <Text style={textStyles.heading}>{tenant.name}</Text>
          <Text style={styles.body}>
            {tenant.slug} • {tenantStatusLabel(tenant.status)}
          </Text>
          {editing?.id === tenant.id ? (
            <>
              <Field
                label="Nama pengelolaan bisnis"
                value={editName}
                onChangeText={setEditName}
                editable={!task.busy}
              />
              <Text style={styles.body}>
                Tidak mengubah identitas struk, QRIS, atau struk transaksi lama.
              </Text>
              <Button
                loading={task.busy}
                disabled={!editName.trim()}
                onPress={() =>
                  void task.run(async () => {
                    await apiRequest(`/management/tenants/${tenant.id}`, {
                      method: "PATCH",
                      token: session.token,
                      body: {
                        name: editName.trim(),
                        expectedRevision: editing.revision ?? 1,
                      },
                    });
                    setEditing(null);
                    await tenants.reload();
                  })
                }
              >
                Simpan nama
              </Button>
              <Button
                variant="ghost"
                disabled={task.busy}
                onPress={() => setEditing(null)}
              >
                Batal
              </Button>
            </>
          ) : (
            <Button
              variant="secondary"
              disabled={task.busy}
              onPress={() => {
                setEditing(tenant);
                setEditName(tenant.name);
              }}
            >
              Ubah nama
            </Button>
          )}
          <Button
            disabled={task.busy}
            variant={tenant.status === "active" ? "danger" : "secondary"}
            onPress={() =>
              Alert.alert(
                tenant.status === "active"
                  ? "Tangguhkan bisnis?"
                  : "Aktifkan bisnis?",
                tenant.status === "active"
                  ? "Akses operasional akan dihentikan. Antrean perangkat offline tetap disimpan dan diamankan saat terhubung kembali."
                  : "Semua akun aktif dapat memilih bisnis ini. Sesi yang telah dicabut tidak diaktifkan kembali.",
                [
                  { text: "Batal", style: "cancel" },
                  {
                    text: "Konfirmasi",
                    onPress: () =>
                      void task.run(async () => {
                        await apiRequest(
                          `/management/tenants/${tenant.id}/status`,
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
                        await tenants.reload();
                      }),
                  },
                ],
              )
            }
          >
            {tenant.status === "active" ? "Tangguhkan" : "Aktifkan"}
          </Button>
        </Card>
      ))}
      <Text style={styles.body}>
        Tenant tidak dapat dihapus. Gunakan penangguhan untuk menghentikan
        operasional tanpa menghilangkan riwayat.
      </Text>
      <Button
        variant="secondary"
        onPress={() => router.push("/management/audit")}
      >
        Riwayat pengelolaan
      </Button>
    </AppScreen>
  );
}

export function ManagedUsersScreen() {
  const router = useRouter();
  const { session } = useAuth();
  const allowed =
    session?.contextKind === "account" && session.user.role === "superadmin";
  const users = useRemote<UserSummary[]>(
    "/management/users",
    allowed ? session.token : undefined,
  );
  const [search, setSearch] = useState("");
  if (!allowed) return <Redirect href="/contexts" />;
  const query = search.trim().toLocaleLowerCase();
  return (
    <AppScreen>
      <PageHeader
        title="Pengguna"
        subtitle="Peran dan status berlaku di seluruh bisnis"
      />
      <Button variant="secondary" onPress={() => router.replace("/contexts")}>
        Ganti bisnis
      </Button>
      <Button
        variant="ghost"
        onPress={() => router.push("/management/tenants")}
      >
        Kelola tenant
      </Button>
      <Field
        label="Cari pengguna"
        value={search}
        onChangeText={setSearch}
        placeholder="Nama atau username"
      />
      <Button onPress={() => router.push("/management/users/new")}>
        Tambah pengguna
      </Button>
      <ErrorMessage message={users.error} />
      <Button variant="secondary" onPress={() => void users.reload()}>
        Muat ulang pengguna
      </Button>
      {users.value
        ?.filter((user) =>
          `${user.fullName} ${user.username}`
            .toLocaleLowerCase()
            .includes(query),
        )
        .map((user) => (
          <Card key={user.id} style={styles.card}>
            <Text style={textStyles.heading}>{user.fullName}</Text>
            <Text style={styles.body}>
              @{user.username} •{" "}
              {user.role === "superadmin" ? "Superadmin" : "Admin"} •{" "}
              {user.active ? "Aktif" : "Nonaktif"}
            </Text>
            <Button
              variant="secondary"
              onPress={() =>
                router.push({
                  pathname: "/management/users/[id]",
                  params: { id: user.id },
                })
              }
            >
              Kelola akun
            </Button>
          </Card>
        ))}
    </AppScreen>
  );
}

export function ManagedUserCreateScreen() {
  const { session } = useAuth();
  const router = useRouter();
  const [fullName, setFullName] = useState("");
  const [username, setUsername] = useState("");
  const [password, setPassword] = useState("");
  const [role, setRole] = useState<Role>("admin");
  const task = useTask();
  if (
    !session ||
    session.contextKind !== "account" ||
    session.user.role !== "superadmin"
  )
    return <Redirect href="/contexts" />;
  return (
    <AppScreen>
      <PageHeader
        back
        title="Tambah pengguna"
        subtitle="Akses otomatis ke semua bisnis aktif"
      />
      <Card style={styles.card}>
        <Field
          label="Nama lengkap"
          value={fullName}
          onChangeText={setFullName}
          editable={!task.busy}
        />
        <Field
          label="Nama pengguna"
          value={username}
          onChangeText={setUsername}
          editable={!task.busy}
          autoCapitalize="none"
          autoCorrect={false}
        />
        <Field
          label="Kata sandi sementara"
          value={password}
          onChangeText={setPassword}
          editable={!task.busy}
          secureTextEntry
          autoComplete="new-password"
        />
        <RolePicker role={role} onChange={setRole} disabled={task.busy} />
        <Text style={styles.body}>
          Bagikan kata sandi secara pribadi. Pengguna wajib menggantinya sebelum
          mulai beroperasi.
        </Text>
        <ErrorMessage message={task.error} />
        <Button
          loading={task.busy}
          disabled={!fullName.trim() || !username.trim() || !password}
          onPress={() =>
            void task.run(async () => {
              await apiRequest("/management/users", {
                method: "POST",
                token: session.token,
                body: {
                  fullName: fullName.trim(),
                  username: username.trim(),
                  role,
                  temporaryPassword: password,
                },
              });
              setPassword("");
              router.replace("/management/users");
            })
          }
        >
          Buat akun
        </Button>
      </Card>
    </AppScreen>
  );
}

export function ManagedUserEditor({ id }: { id: string }) {
  const { session } = useAuth();
  const allowed =
    session?.contextKind === "account" && session.user.role === "superadmin";
  const user = useRemote<UserSummary>(
    `/management/users/${encodeURIComponent(id)}`,
    allowed ? session.token : undefined,
  );
  const [draftRole, setDraftRole] = useState<Role | null>(null);
  const [draftActive, setDraftActive] = useState<boolean | null>(null);
  const [password, setPassword] = useState("");
  const [message, setMessage] = useState<string | null>(null);
  const task = useTask();
  if (!allowed) return <Redirect href="/contexts" />;
  const member = user.value;
  const self = session.user.id === id;
  const role = draftRole ?? member?.role ?? "admin";
  const active = draftActive ?? member?.active ?? false;
  return (
    <AppScreen>
      <PageHeader
        back
        title="Kelola akun"
        subtitle="Perubahan berlaku untuk seluruh bisnis"
      />
      <ErrorMessage message={task.error ?? user.error} />
      {message ? (
        <Text accessibilityRole="alert" style={styles.body}>
          {message}
        </Text>
      ) : null}
      {!member ? (
        <Button variant="secondary" onPress={() => void user.reload()}>
          Muat ulang akun
        </Button>
      ) : (
        <Card style={styles.card}>
          <Text style={textStyles.heading}>{member.fullName}</Text>
          <Text style={styles.body}>@{member.username}</Text>
          <RolePicker
            role={role}
            onChange={setDraftRole}
            disabled={self || task.busy}
          />
          <Button
            variant="secondary"
            disabled={self || task.busy}
            onPress={() => setDraftActive(!active)}
          >
            {active
              ? "Aktif — ketuk untuk nonaktifkan"
              : "Nonaktif — ketuk untuk aktifkan"}
          </Button>
          <Text style={styles.body}>
            {self
              ? "Anda tidak dapat menurunkan peran atau menonaktifkan akun sendiri. Gunakan Profil untuk mengganti kata sandi."
              : "Perubahan peran/status mencabut sesi akun di semua bisnis. Antrean offline tetap disimpan."}
          </Text>
          {!self ? (
            <>
              <Button
                loading={task.busy}
                disabled={role === member.role && active === member.active}
                onPress={() =>
                  Alert.alert(
                    "Simpan akses global?",
                    "Akun perlu masuk kembali di semua perangkat.",
                    [
                      { text: "Batal", style: "cancel" },
                      {
                        text: "Simpan",
                        onPress: () =>
                          void task.run(async () => {
                            await apiRequest(`/management/users/${id}`, {
                              method: "PATCH",
                              token: session.token,
                              body: { role, active },
                            });
                            setDraftRole(null);
                            setDraftActive(null);
                            setMessage("Akses akun diperbarui.");
                            await user.reload();
                          }),
                      },
                    ],
                  )
                }
              >
                Simpan akses akun
              </Button>
              <Field
                label="Kata sandi sementara baru"
                secureTextEntry
                value={password}
                onChangeText={setPassword}
                editable={!task.busy}
              />
              <Button
                variant="danger"
                loading={task.busy}
                disabled={!password}
                onPress={() =>
                  Alert.alert(
                    "Reset kata sandi?",
                    "Semua sesi akun dicabut. Pengguna wajib mengganti kata sandi sementara setelah masuk.",
                    [
                      { text: "Batal", style: "cancel" },
                      {
                        text: "Reset",
                        onPress: () =>
                          void task.run(async () => {
                            await apiRequest(
                              `/management/users/${id}/reset-password`,
                              {
                                method: "POST",
                                token: session.token,
                                body: { temporaryPassword: password },
                              },
                            );
                            setPassword("");
                            setMessage(
                              "Kata sandi direset. Bagikan kata sandi sementara secara pribadi.",
                            );
                            await user.reload();
                          }),
                      },
                    ],
                  )
                }
              >
                Reset kata sandi
              </Button>
            </>
          ) : null}
        </Card>
      )}
    </AppScreen>
  );
}

interface ManagementAudit {
  id: string;
  eventType: string;
  createdAt: string;
}
export function ManagementAuditScreen() {
  const { session } = useAuth();
  const allowed =
    session?.contextKind === "account" && session.user.role === "superadmin";
  const audit = useRemote<ManagementAudit[]>(
    "/management/audit",
    allowed ? session.token : undefined,
  );
  if (!allowed) return <Redirect href="/contexts" />;
  return (
    <AppScreen>
      <PageHeader
        back
        title="Riwayat pengelolaan"
        subtitle="Aktivitas administratif seluruh organisasi"
      />
      <ErrorMessage message={audit.error} />
      <Button variant="secondary" onPress={() => void audit.reload()}>
        Muat ulang riwayat
      </Button>
      {audit.value?.map((event) => (
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

export function BusinessProfileScreen() {
  const { session } = useAuth();
  const allowed =
    session?.user.role === "superadmin" && session.dataMode === "production";
  const remote = useRemote<BusinessProfile>(
    "/tenant/profile",
    allowed ? session.token : undefined,
  );
  const [draft, setDraft] = useState<BusinessProfile | null>(null);
  const [saved, setSaved] = useState(false);
  const task = useTask();
  if (!allowed) return <Redirect href="/" />;
  const profile = draft ?? remote.value;
  return (
    <AppScreen>
      <PageHeader
        back
        title="Identitas struk"
        subtitle="Digunakan pada transaksi baru, struk, dan laporan"
      />
      <ErrorMessage message={task.error ?? remote.error} />
      {profile ? (
        <Card style={styles.card}>
          <Field
            label="Nama bisnis"
            value={profile.businessName}
            onChangeText={(businessName) => {
              setSaved(false);
              setDraft({ ...profile, businessName });
            }}
          />
          <Field
            label="Alamat (opsional)"
            value={profile.address ?? ""}
            onChangeText={(address) => {
              setSaved(false);
              setDraft({ ...profile, address });
            }}
          />
          <Field
            label="Telepon (opsional)"
            keyboardType="phone-pad"
            value={profile.phone ?? ""}
            onChangeText={(phone) => {
              setSaved(false);
              setDraft({ ...profile, phone });
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
                setDraft(value);
                setSaved(true);
              })
            }
          >
            Simpan identitas
          </Button>
          {saved ? (
            <Text style={styles.body}>
              Identitas tersimpan. Nama pengelolaan bisnis dan struk transaksi
              lama tidak berubah.
            </Text>
          ) : null}
        </Card>
      ) : (
        <Button variant="secondary" onPress={() => void remote.reload()}>
          Muat ulang identitas
        </Button>
      )}
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
