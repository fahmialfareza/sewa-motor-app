import { useLocalSearchParams } from "expo-router";
import { MembershipEditor } from "@/tenant/screens";
export default function EditUserScreen() {
  const { id } = useLocalSearchParams<{ id: string }>();
  return <MembershipEditor id={id} />;
}
