import {
  DISPLAY_NAME_MAX_LENGTH,
  ORG_DISPLAY_NAME_MAX_LENGTH,
} from "@/lib/constants";

export type AccountDisplayNameKind = "personal" | "organization";

const DISPLAY_NAME_LIMIT_LABELS: Record<AccountDisplayNameKind, string> = {
  personal: "Display names",
  organization: "Organization names",
};

const DISPLAY_NAME_EMPTY_MESSAGES: Record<AccountDisplayNameKind, string> = {
  personal: "Display name can't be empty",
  organization: "Organization name can't be empty",
};

const DISPLAY_NAME_REQUIRED: Record<AccountDisplayNameKind, boolean> = {
  personal: true,
  organization: true,
};

export function getDisplayNameMaxLength(kind: AccountDisplayNameKind) {
  return kind === "organization"
    ? ORG_DISPLAY_NAME_MAX_LENGTH
    : DISPLAY_NAME_MAX_LENGTH;
}

export function isDisplayNameRequired(kind: AccountDisplayNameKind) {
  return DISPLAY_NAME_REQUIRED[kind];
}

export function getDisplayNameEmptyError(kind: AccountDisplayNameKind) {
  return DISPLAY_NAME_EMPTY_MESSAGES[kind];
}

export function getDisplayNameError(
  displayName: string,
  kind: AccountDisplayNameKind,
) {
  if (isDisplayNameRequired(kind) && displayName.trim() === "") {
    return getDisplayNameEmptyError(kind);
  }

  return getDisplayNameValidationError(displayName, kind);
}

export function getDisplayNameValidationError(
  displayName: string,
  kind: AccountDisplayNameKind,
) {
  const maxLength = getDisplayNameMaxLength(kind);

  if ([...displayName.trim()].length <= maxLength) {
    return null;
  }

  return `${DISPLAY_NAME_LIMIT_LABELS[kind]} cannot exceed ${maxLength} characters.`;
}
