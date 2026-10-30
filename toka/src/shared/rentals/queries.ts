import { unwrapApiData } from "../services/api";
import { rentalService, type RentalDTO } from "../services/rental.service";

export const rentalsListQueryKey = (params: Parameters<typeof rentalService.list>[0] = {}) => ["rentals", params] as const;

export const rentalQueryKey = (id: string) => ["rental", id] as const;

export const fetchRentalsList = async (params: Parameters<typeof rentalService.list>[0] = {}): Promise<RentalDTO[]> => {
  const res = await rentalService.list(params);
  return unwrapApiData<RentalDTO[]>(res);
};

export const fetchRental = async (id: string): Promise<RentalDTO> => {
  const res = await rentalService.getById(id);
  return unwrapApiData<RentalDTO>(res);
};
