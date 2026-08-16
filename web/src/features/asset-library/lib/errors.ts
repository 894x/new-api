/*
Copyright (C) 2023-2026 QuantumNous

This program is free software: you can redistribute it and/or modify
it under the terms of the GNU Affero General Public License as
published by the Free Software Foundation, either version 3 of the
License, or (at your option) any later version.

This program is distributed in the hope that it will be useful,
but WITHOUT ANY WARRANTY; without even the implied warranty of
MERCHANTABILITY or FITNESS FOR A PARTICULAR PURPOSE. See the
GNU Affero General Public License for more details.

You should have received a copy of the GNU Affero General Public License
along with this program. If not, see <https://www.gnu.org/licenses/>.

For commercial licensing, please contact support@quantumnous.com
*/
export function getAssetLibraryErrorMessage(
  error: unknown,
  fallback: string
): string {
  const responseData = (
    error as {
      response?: {
        data?: {
          ResponseMetadata?: {
            Error?: { Code?: string; Message?: string }
          }
          message?: string
          error?: { message?: string }
        }
      }
    }
  ).response?.data
  const assetLibraryError = responseData?.ResponseMetadata?.Error
  if (assetLibraryError?.Code && assetLibraryError.Message) {
    return `${assetLibraryError.Code}: ${assetLibraryError.Message}`
  }
  if (assetLibraryError?.Message || assetLibraryError?.Code) {
    return assetLibraryError.Message || assetLibraryError.Code || fallback
  }

  if (responseData?.message) return responseData.message
  if (responseData?.error?.message) return responseData.error.message
  if (error instanceof Error && error.message) return error.message
  return fallback
}
