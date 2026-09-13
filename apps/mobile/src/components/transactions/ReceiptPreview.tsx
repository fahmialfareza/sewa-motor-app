import { ScrollView, StyleSheet, Text } from "react-native";

import { Card } from "@/components/ui/Card";
import { formatReceipt } from "@/printer/receipt";
import type { ReceiptDocument } from "@/printer/types";
import { colors, spacing, textStyles, typography } from "@/theme/tokens";

interface ReceiptPreviewProps {
  document: ReceiptDocument;
  columns: 32 | 48;
}

/** Read-only: this component never connects a printer or records an attempt. */
export function ReceiptPreview({ document, columns }: ReceiptPreviewProps) {
  return (
    <Card style={styles.card} testID="receipt-preview">
      <Text style={textStyles.heading}>Pratinjau struk</Text>
      <Text style={styles.caption}>
        {columns} kolom · Pratinjau tidak mencetak atau mengubah status cetak.
      </Text>
      <ScrollView
        nestedScrollEnabled
        style={styles.viewport}
        accessibilityLabel="Gulir pratinjau struk"
      >
        <ScrollView
          horizontal
          contentContainerStyle={styles.paper}
          accessibilityLabel="Geser lebar struk"
        >
          <Text selectable style={styles.receipt} testID="receipt-preview-text">
            {formatReceipt(document, columns)}
          </Text>
        </ScrollView>
      </ScrollView>
    </Card>
  );
}

const styles = StyleSheet.create({
  card: { gap: spacing.sm },
  caption: { ...textStyles.body, color: colors.textMuted },
  viewport: { maxHeight: 360 },
  paper: { padding: spacing.sm },
  receipt: {
    fontFamily: typography.mono,
    fontSize: 12,
    lineHeight: 18,
    letterSpacing: 0,
    color: colors.text,
    flexShrink: 0,
  },
});
