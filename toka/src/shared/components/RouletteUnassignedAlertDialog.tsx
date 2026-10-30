import {
  AlertDialog,
  AlertDialogAction,
  AlertDialogCancel,
  AlertDialogContent,
  AlertDialogDescription,
  AlertDialogFooter,
  AlertDialogHeader,
  AlertDialogTitle,
} from "../ui/alert-dialog";
import { Button } from "../ui/button";

type Props = {
  open: boolean;
  count: number;
  isLoading?: boolean;
  onAssignAutomatically: () => void;
  onKeepManual: () => void;
  onCancel: () => void;
};

export function RouletteUnassignedAlertDialog({
  open,
  count,
  isLoading,
  onAssignAutomatically,
  onKeepManual,
  onCancel,
}: Props) {
  const label = `${count} lead${count === 1 ? "" : "s"}`;

  return (
    <AlertDialog open={open} onOpenChange={(v) => (!v ? onCancel() : undefined)}>
      <AlertDialogContent>
        <AlertDialogHeader>
          <AlertDialogTitle>Leads sem corretor</AlertDialogTitle>
          <AlertDialogDescription>
            Existem {label} sem corretor. Como deseja proceder ao ativar a roleta?
          </AlertDialogDescription>
        </AlertDialogHeader>
        <AlertDialogFooter>
          <AlertDialogCancel asChild>
            <Button variant="outline" disabled={!!isLoading}>
              Cancelar
            </Button>
          </AlertDialogCancel>
          <AlertDialogAction asChild>
            <Button variant="secondary" disabled={!!isLoading} onClick={onKeepManual}>
              Manter manual
            </Button>
          </AlertDialogAction>
          <AlertDialogAction asChild>
            <Button disabled={!!isLoading} onClick={onAssignAutomatically}>
              Atribuir automaticamente
            </Button>
          </AlertDialogAction>
        </AlertDialogFooter>
      </AlertDialogContent>
    </AlertDialog>
  );
}
