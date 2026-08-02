import { createContext, useCallback, useContext, useState } from "react";
import { AlertTriangle } from "lucide-react";
import Modal from "@/components/Modal";

export interface ConfirmOptions {
  title?: string;
  message: React.ReactNode;
  confirmText?: string;
  cancelText?: string;
  /** 破坏性操作使用红色确认按钮 */
  danger?: boolean;
}

type ConfirmFn = (opts: ConfirmOptions) => Promise<boolean>;

const ConfirmContext = createContext<ConfirmFn>(async () => false);

/**
 * 统一风格确认对话框（替代浏览器原生 confirm）。
 *
 * 用法：
 *   const confirmDialog = useConfirm();
 *   if (!(await confirmDialog({ title: "确认删除", message: "...", danger: true }))) return;
 */
export function ConfirmProvider({ children }: { children: React.ReactNode }) {
  const [state, setState] = useState<{ opts: ConfirmOptions; resolve: (v: boolean) => void } | null>(null);

  const confirm = useCallback<ConfirmFn>((opts) => {
    return new Promise<boolean>((resolve) => {
      setState({ opts, resolve });
    });
  }, []);

  const close = useCallback(
    (result: boolean) => {
      state?.resolve(result);
      setState(null);
    },
    [state]
  );

  return (
    <ConfirmContext.Provider value={confirm}>
      {children}
      {state && (
        <Modal open onClose={() => close(false)} title={state.opts.title ?? "确认操作"} width="max-w-sm">
          <div className="flex items-start gap-3">
            <AlertTriangle
              className={`mt-0.5 h-5 w-5 shrink-0 ${
                state.opts.danger ? "text-red-400" : "text-amber-400"
              }`}
            />
            <div className="text-sm leading-relaxed text-zinc-300">{state.opts.message}</div>
          </div>
          <div className="mt-6 flex justify-end gap-2">
            <button
              onClick={() => close(false)}
              className="rounded-md border border-zinc-700 px-4 py-1.5 text-sm text-zinc-300 hover:bg-zinc-800"
            >
              {state.opts.cancelText ?? "取消"}
            </button>
            <button
              onClick={() => close(true)}
              autoFocus
              className={`rounded-md px-4 py-1.5 text-sm font-medium ${
                state.opts.danger
                  ? "bg-red-600 text-white hover:bg-red-500"
                  : "bg-amber-500 text-black hover:bg-amber-400"
              }`}
            >
              {state.opts.confirmText ?? "确认"}
            </button>
          </div>
        </Modal>
      )}
    </ConfirmContext.Provider>
  );
}

export function useConfirm() {
  return useContext(ConfirmContext);
}
