export const publicAsset = (assetPath: string) => `${import.meta.env.BASE_URL}${assetPath.replace(/^\/+/, '')}`;

export const backgroundAsset = (fileName: string) => publicAsset(`ui-assets/backgrounds/${fileName}`);
