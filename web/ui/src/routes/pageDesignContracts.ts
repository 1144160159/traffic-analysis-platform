import registry from '@/routes/pageDesignContracts.v1.json';

export type PageDesignContract = (typeof registry.pages)[number];
export type PageDesignContractRegistry = typeof registry;

export const pageDesignContractRegistry: PageDesignContractRegistry = registry;

export const findPageDesignContract = (pageId: string) =>
  pageDesignContractRegistry.pages.find((contract) => contract.page_id === pageId);

export const findCompatibilityAlias = (routeId: string) =>
  pageDesignContractRegistry.compatibility_aliases.find((alias) => alias.route_id === routeId);
