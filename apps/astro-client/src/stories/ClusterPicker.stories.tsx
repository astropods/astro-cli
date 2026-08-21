import type { Meta, StoryObj } from "@storybook/react-vite";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import { ClusterPicker } from "@/components/deploy/ClusterPicker";
import { accountKeys } from "@/api/queries/keys";
import type { AllowedCluster } from "@/lib/api";

const ACCOUNT = "acme";

function seeded(allowed: AllowedCluster[]) {
  const client = new QueryClient({
    defaultOptions: { queries: { retry: false, staleTime: Infinity } },
  });
  client.setQueryData(accountKeys.detail(ACCOUNT), {
    id: "acct-1",
    name: ACCOUNT,
    type: "organization",
    created_at: "",
    updated_at: "",
    social_links: [],
    allowed_clusters: allowed,
  });
  return client;
}

const ireland: AllowedCluster = {
  cluster_id: "prod-eu-west-1",
  region: "eu-west-1",
  region_label: "Europe (Ireland)",
  region_flag: "🇮🇪",
  is_default: true,
};
const virginia: AllowedCluster = {
  cluster_id: "prod-us-east-1",
  region: "us-east-1",
  region_label: "US East (N. Virginia)",
  region_flag: "🇺🇸",
};
const tokyo: AllowedCluster = {
  cluster_id: "prod-ap-northeast-1",
  region: "ap-northeast-1",
  region_label: "Asia Pacific (Tokyo)",
  region_flag: "🇯🇵",
};

function Harness({ allowed, ...props }: { allowed: AllowedCluster[] } & Partial<{ value: string; readOnly: boolean }>) {
  return (
    <QueryClientProvider client={seeded(allowed)}>
      <div className="max-w-[32rem] p-6">
        <ClusterPicker account={ACCOUNT} value="" onChange={() => {}} {...props} />
      </div>
    </QueryClientProvider>
  );
}

const meta: Meta<typeof Harness> = {
  title: "Deploy/ClusterPicker",
  component: Harness,
};
export default meta;

type Story = StoryObj<typeof Harness>;

export const SingleRegion: Story = {
  args: { allowed: [ireland] },
};

export const MultipleRegions: Story = {
  args: { allowed: [ireland, virginia, tokyo] },
};

export const SecondRegionSelected: Story = {
  args: { allowed: [ireland, virginia, tokyo], value: "prod-us-east-1" },
};

export const ReadOnlyOnConfigure: Story = {
  args: { allowed: [ireland, virginia], readOnly: true },
};
