import { useLocalSearchParams } from "expo-router";
import { ManagedUserEditor } from "@/tenant/screens";
export default function EditManagedUserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <ManagedUserEditor key={id} id={id} />;
}
