import MaterialCommunityIcons from "@expo/vector-icons/MaterialCommunityIcons";
import type { ComponentProps } from "react";

type IconName = ComponentProps<typeof MaterialCommunityIcons>["name"];

interface IconProps {
  name: IconName;
  color?: string;
  size?: number;
}

export function Icon({ name, color, size = 22 }: IconProps) {
  return <MaterialCommunityIcons color={color} name={name} size={size} />;
}

export type { IconName };
