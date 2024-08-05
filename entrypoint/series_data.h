#ifdef __cplusplus
extern "C" {
#endif

void prompp_series_data_data_storage_ctor(void* res);
void prompp_series_data_data_storage_dtor(void* args);
void prompp_series_data_encoder_ctor(void* args, void* res);
void prompp_series_data_encoder_encode(void* args);
void prompp_series_data_encoder_encode_inner_series_slice(void* args);
void prompp_series_data_encoder_dtor(void* args);

void prompp_series_data_querier_ctor(void* args, void* res);
void prompp_series_data_querier_dtor(void* args);

#ifdef __cplusplus
}  // extern "C"
#endif
