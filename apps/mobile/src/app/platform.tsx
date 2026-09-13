import { Redirect } from "expo-router";
// Retain old deep links without offering a retired context.
export default function LegacyManagementRoute() {
  return <Redirect href="/contexts" />;
}
