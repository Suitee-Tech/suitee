// Standard API Response Interface
export interface ApiResponse<T> {
  success: boolean;
  message: string;
  statusCode: number;
  data: T;
  meta?: PaginationMeta;
}

// Pagination Metadata
export interface PaginationMeta {
  page: number;
  limit: number;
  total: number;
  totalPages: number;
}

// Property Interface (matching Go struct)
export interface Property {
  id: string;
  external_id?: string;
  title: string;
  description: string;
  status: string;
  publication_status: string;
  property_type: string;
  listing_type: string;
  condition: string;
  address: Address;
  amenities: string[];
  tags: string[];
  // Add other fields as needed
}

export interface Address {
  street: string;
  number: string;
  complement?: string;
  neighborhood: string;
  district?: string;
  city: string;
  state: string;
  state_code: string;
  postal_code: string;
  country: string;
  country_code: string;
  is_verified: boolean;
  longitude: number;
  latitude: number;
  formatted_address: string;
  reference_point?: string;
}
